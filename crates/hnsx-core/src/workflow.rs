//! `WorkflowEngine`: executes a `Workflow` over a set of agents, yielding
//! a stream of `Chunk`s.
//!
//! Phase 1.1+ scope:
//! - DAG execution with explicit success (`next`) and failure (`on_error`) edges.
//! - `output` binding: each step's aggregated text is exposed as
//!   `steps.<step_id>.output`.
//! - Input rendering: recursive `${...}` substitution for
//!   `trigger.<path>`, `steps.<id>.output`, `variables.<name>`, array indices,
//!   and `| default("...")` filters.
//! - `condition`: optional template; step runs iff the rendered value is
//!   non-empty and not the literal `"false"`.
//! - Step-level `timeout_seconds` and `retry` with exponential backoff.
//! - Workflow-level `error_policy`: `fail_fast` (default), `continue`,
//!   `fallback_step`.
//! - PII detection on text output.

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use async_stream::stream;
use futures::stream::{BoxStream, StreamExt};
use petgraph::algo::toposort;
use petgraph::graph::{DiGraph, NodeIndex};
use petgraph::visit::EdgeRef;
use serde_json::Value;
use uuid::Uuid;

use crate::agent::{Agent, InvokeContext};
use crate::chunk::Chunk;
use crate::domain::{ErrorPolicy, Step, Workflow};
use crate::error::{Error, Result};
use crate::memory::MemoryBackend;
use crate::telemetry::{StepTrace, Telemetry, now_ms};

/// Outcome of a single step: concatenated text + all chunks received.
#[derive(Debug, Default, Clone)]
pub struct StepResult {
    /// Concatenation of every `Chunk::Text` yielded by the agent.
    pub output: String,
    /// Full chunk history (for debugging / replay).
    pub chunks: Vec<Chunk>,
    /// Whether the step succeeded.
    pub success: bool,
}

/// Mutable state carried through workflow execution.
#[derive(Debug, Default, Clone)]
pub struct ExecutionContext {
    /// The original trigger payload.
    pub trigger: Value,
    /// Aggregated per-step results, keyed by step id.
    pub steps: HashMap<String, StepResult>,
    /// Workflow-level variables (from `Workflow.variables`).
    pub variables: HashMap<String, Value>,
}

impl ExecutionContext {
    pub fn new(trigger: Value, variables: HashMap<String, Value>) -> Self {
        Self {
            trigger,
            steps: HashMap::new(),
            variables,
        }
    }
}

/// A pre-built workflow engine. Cheap to clone (everything inside is `Arc`/owned).
#[derive(Clone)]
pub struct WorkflowEngine {
    steps: Vec<Step>,
    agents: HashMap<String, Arc<dyn Agent>>,
    /// step id -> index in `steps`.
    step_index: HashMap<String, usize>,
    /// Graph edges: success edges from `next` or default order; failure edges from `on_error`.
    graph: DiGraph<String, EdgeKind>,
    /// Map step id -> graph node index.
    node_index: HashMap<String, NodeIndex>,
    entry: String,
    error_policy: ErrorPolicy,
    telemetry: Option<Arc<Telemetry>>,
    domain_id: String,
    memory: Option<Arc<dyn MemoryBackend>>,
    default_memory_window: usize,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum EdgeKind {
    Success,
    Failure,
}

impl WorkflowEngine {
    /// Build an engine from a parsed `Workflow` and an agent map.
    pub fn new(workflow: Workflow, agents: HashMap<String, Arc<dyn Agent>>) -> Result<Self> {
        Self::new_full(
            workflow,
            agents,
            None,
            String::new(),
            None,
            10,
        )
    }

    /// Build an engine that also writes per-step traces to `telemetry`.
    pub fn new_with_telemetry(
        workflow: Workflow,
        agents: HashMap<String, Arc<dyn Agent>>,
        telemetry: Option<Arc<Telemetry>>,
        domain_id: String,
    ) -> Result<Self> {
        Self::new_full(
            workflow,
            agents,
            telemetry,
            domain_id,
            None,
            10,
        )
    }

    /// Build an engine with memory backend.
    pub fn new_with_memory(
        workflow: Workflow,
        agents: HashMap<String, Arc<dyn Agent>>,
        telemetry: Option<Arc<Telemetry>>,
        domain_id: String,
        memory: Arc<dyn MemoryBackend>,
        default_memory_window: usize,
    ) -> Result<Self> {
        Self::new_full(
            workflow,
            agents,
            telemetry,
            domain_id,
            Some(memory),
            default_memory_window,
        )
    }

    fn new_full(
        workflow: Workflow,
        agents: HashMap<String, Arc<dyn Agent>>,
        telemetry: Option<Arc<Telemetry>>,
        domain_id: String,
        memory: Option<Arc<dyn MemoryBackend>>,
        default_memory_window: usize,
    ) -> Result<Self> {
        if workflow.steps.is_empty() {
            return Err(Error::InvalidSpec("workflow has no steps".into()));
        }

        let mut step_index: HashMap<String, usize> =
            HashMap::with_capacity(workflow.steps.len());
        for (i, step) in workflow.steps.iter().enumerate() {
            step_index.insert(step.id.clone(), i);
        }

        if !step_index.contains_key(&workflow.entry) {
            return Err(Error::InvalidSpec(format!(
                "workflow.entry '{}' does not match any step id",
                workflow.entry
            )));
        }

        // Validate step agent references.
        for step in &workflow.steps {
            if !agents.contains_key(&step.agent) {
                return Err(Error::InvalidSpec(format!(
                    "step '{}' references unknown agent '{}'",
                    step.id, step.agent
                )));
            }
        }

        // Build graph.
        let mut graph: DiGraph<String, EdgeKind> = DiGraph::new();
        let mut node_index: HashMap<String, NodeIndex> = HashMap::new();
        for step in &workflow.steps {
            let idx = graph.add_node(step.id.clone());
            node_index.insert(step.id.clone(), idx);
        }

        for (i, step) in workflow.steps.iter().enumerate() {
            let from = node_index[&step.id];

            // Success edge.
            let target = step
                .next
                .as_ref()
                .and_then(|id| step_index.get(id).copied())
                .unwrap_or(i + 1);
            if target < workflow.steps.len() {
                let to = node_index[&workflow.steps[target].id];
                graph.add_edge(from, to, EdgeKind::Success);
            }

            // Failure edge.
            if let Some(err_id) = &step.on_error {
                if let Some(&err_idx) = step_index.get(err_id) {
                    let to = node_index[&workflow.steps[err_idx].id];
                    graph.add_edge(from, to, EdgeKind::Failure);
                } else {
                    return Err(Error::InvalidSpec(format!(
                        "step '{}' on_error references unknown step '{}'",
                        step.id, err_id
                    )));
                }
            }
        }

        // Cycle detection.
        if toposort(&graph, None).is_err() {
            return Err(Error::InvalidSpec("workflow graph has a cycle".into()));
        }

        Ok(Self {
            steps: workflow.steps,
            agents,
            step_index,
            graph,
            node_index,
            entry: workflow.entry,
            error_policy: workflow.error_policy,
            telemetry,
            domain_id,
            memory,
            default_memory_window,
        })
    }

    /// Execute the workflow against `trigger`, returning a chunk stream.
    #[tracing::instrument(skip(self, trigger), fields(domain_id = %self.domain_id))]
    pub fn execute(&self, trigger: Value) -> BoxStream<'static, Chunk> {
        let engine = self.clone();
        Box::pin(stream! {
            let workflow_started_at_ms = now_ms();
            let mut total_prompt_tokens: u64 = 0;
            let mut total_completion_tokens: u64 = 0;
            let mut total_cost_usd: f64 = 0.0;

            // Resolve session id from trigger or generate a new one.
            let session_id = trigger
                .get("session_id")
                .and_then(Value::as_str)
                .map(String::from)
                .unwrap_or_else(|| Uuid::new_v4().to_string());

            let mut session = if let Some(mem) = engine.memory.as_ref() {
                match mem.load_session(&engine.domain_id, &session_id).await {
                    Ok(s) => Some(s),
                    Err(e) => {
                        yield Chunk::error(format!("failed to load session: {e}"));
                        return;
                    }
                }
            } else {
                None
            };

            let mut ctx = ExecutionContext::new(trigger.clone(), engine.workflow_variables());
            let mut current = Some(engine.node_index[&engine.entry]);

            while let Some(node) = current {
                let step_index = *engine.step_index.get(&engine.graph[node]).unwrap();
                let step = engine.steps[step_index].clone();

                // 1. Condition gate.
                if let Some(cond) = &step.condition {
                    let rendered = render_template_string(cond, &ctx);
                    let run = !rendered.is_empty() && rendered != "false";
                    if !run {
                        current = engine.successor(node, true);
                        continue;
                    }
                }

                // 2. Resolve agent.
                let agent = match engine.agents.get(&step.agent) {
                    Some(a) => a.clone(),
                    None => {
                        yield Chunk::error(format!(
                            "step '{}': unknown agent '{}'",
                            step.id, step.agent
                        ));
                        return;
                    }
                };

                // 3. Render input.
                let mut input = render_value(&step.input, &ctx);

                // 4. Inject memory context if available.
                if let (Some(mem), Some(session)) = (engine.memory.as_ref(), session.as_ref()) {
                    let window = engine.default_memory_window;
                    match mem.build_context(session, &step.agent, window).await {
                        Ok(messages) if !messages.is_empty() => {
                            if let Value::Object(ref mut map) = input {
                                let mem_val = serde_json::to_value(&messages).unwrap_or_default();
                                map.insert("_memory".to_string(), mem_val);
                            }
                        }
                        Ok(_) => {}
                        Err(e) => {
                            yield Chunk::error(format!("memory build_context failed: {e}"));
                            return;
                        }
                    }
                }

                // 5. Invoke agent with timeout and retry.
                let invoke_ctx = InvokeContext {
                    session_id: session_id.clone(),
                    domain_id: engine.domain_id.clone(),
                    agent_id: step.agent.clone(),
                };

                let started_at_ms = now_ms();
                let timeout = Duration::from_secs(step.timeout_seconds.unwrap_or(300));
                let retry = step.retry.unwrap_or_default();

                let mut result = StepResult::default();
                let mut failed_with: Option<String> = None;

                for attempt in 0..=retry.count {
                    if attempt > 0 {
                        let backoff = Duration::from_millis(retry.backoff_ms * 2_u64.pow(attempt - 1));
                        tokio::time::sleep(backoff).await;
                    }

                    let invoke_fut = agent.invoke(input.clone(), invoke_ctx.clone());
                    let mut stream = match tokio::time::timeout(timeout, invoke_fut).await {
                        Ok(Ok(s)) => s,
                        Ok(Err(e)) => {
                            failed_with = Some(format!("{e}"));
                            continue;
                        }
                        Err(_) => {
                            failed_with = Some(format!("step '{}' timed out after {}s", step.id, timeout.as_secs()));
                            continue;
                        }
                    };

                    result = StepResult::default();
                    while let Some(chunk) = stream.next().await {
                        if let Chunk::Text(t) = &chunk {
                            if crate::pii::contains_pii(t) {
                                yield Chunk::error("PII detected in agent output; stream halted".to_string());
                                return;
                            }
                            result.output.push_str(t);
                        }
                        if let Chunk::Artifact(crate::chunk::Artifact::TokenUsage { prompt, completion, cost_usd }) = &chunk {
                            total_prompt_tokens += prompt;
                            total_completion_tokens += completion;
                            total_cost_usd += cost_usd;
                        }
                        result.chunks.push(chunk.clone());
                        if step.stream {
                            yield chunk;
                        }
                    }
                    failed_with = None;
                    result.success = true;
                    break;
                }

                // 6. Handle failure.
                if let Some(err) = failed_with {
                    result.success = false;
                    if !step.stream {
                        yield Chunk::error(err.clone());
                    }

                    // Emit telemetry for failed step too.
                    engine.record_step(&step, &session_id, started_at_ms, input.clone(), result.output.clone()).await;

                    // Save a turn with the error as assistant output for observability.
                    if let (Some(mem), Some(ref mut session)) = (engine.memory.as_ref(), session.as_mut()) {
                        let _ = mem.save_turn(session, &step.agent, "assistant", &format!("ERROR: {err}")).await;
                    }

                    ctx.steps.insert(step.id.clone(), result);

                    // Failure edge has priority.
                    if let Some(next) = engine.failure_successor(node) {
                        current = Some(next);
                        continue;
                    }

                    match &engine.error_policy {
                        ErrorPolicy::FailFast => {
                            yield Chunk::error(format!("workflow failed at step '{}': {err}", step.id));
                            return;
                        }
                        ErrorPolicy::Continue => {
                            current = engine.successor(node, false);
                            continue;
                        }
                        ErrorPolicy::FallbackStep(fallback) => {
                            current = engine.node_index.get(fallback).copied();
                            continue;
                        }
                    }
                }

                // 7. Successful step: telemetry + memory + context.
                engine.record_step(&step, &session_id, started_at_ms, input.clone(), result.output.clone()).await;

                if let (Some(mem), Some(ref mut session)) = (engine.memory.as_ref(), session.as_mut()) {
                    if let Err(e) = mem.save_turn(session, &step.agent, "assistant", &result.output).await {
                        yield Chunk::error(format!("memory save_turn failed: {e}"));
                        return;
                    }
                }

                ctx.steps.insert(step.id.clone(), result);
                current = engine.successor(node, true);
            }

            // 8. Final Chunk::done carries the step outputs as a flat object.
            let mut done_vars = serde_json::Map::new();
            for (k, v) in &ctx.steps {
                done_vars.insert(format!("steps.{k}.output"), Value::String(v.output.clone()));
            }
            done_vars.insert("session_id".to_string(), Value::String(session_id.clone()));
            yield Chunk::done(Value::Object(done_vars));

            // 9. Report invocation-level telemetry.
            if let Some(tel) = engine.telemetry.as_ref() {
                let duration_ms = now_ms().saturating_sub(workflow_started_at_ms);
                tel.record_invocation(&hnsx_proto::v1::InvocationRecord {
                    session_id: session_id.clone(),
                    domain_id: engine.domain_id.clone(),
                    started_at_ms: workflow_started_at_ms as i64,
                    duration_ms: duration_ms as i64,
                    prompt_tokens: total_prompt_tokens as i64,
                    completion_tokens: total_completion_tokens as i64,
                    total_cost_usd,
                });
            }
        })
    }

    /// Find the success successor of a node. `skip` means the step was skipped via condition.
    fn successor(&self, node: NodeIndex, _skipped: bool) -> Option<NodeIndex> {
        // If skipped, follow the first success edge (which normally means "next").
        // We do not follow failure edges for skipped steps.
        self.graph
            .edges(node)
            .find(|e| *e.weight() == EdgeKind::Success)
            .map(|e| e.target())
    }

    fn failure_successor(&self, node: NodeIndex) -> Option<NodeIndex> {
        self.graph
            .edges(node)
            .find(|e| *e.weight() == EdgeKind::Failure)
            .map(|e| e.target())
    }

    async fn record_step(
        &self,
        step: &Step,
        session_id: &str,
        started_at_ms: u64,
        input: Value,
        output: String,
    ) {
        if let Some(tel) = self.telemetry.as_ref() {
            let trace = StepTrace {
                session_id: session_id.to_string(),
                domain_id: self.domain_id.clone(),
                step_id: step.id.clone(),
                agent_id: step.agent.clone(),
                started_at_ms,
                duration_ms: now_ms().saturating_sub(started_at_ms),
                input,
                output,
            };
            if let Err(e) = tel.record_step(&trace) {
                tracing::warn!(error = %e, "failed to record step trace");
            }
        }
    }

    fn workflow_variables(&self) -> HashMap<String, Value> {
        HashMap::new()
    }
}

// ---------------------------------------------------------------------------
// Template rendering: ${trigger.x.y}, ${steps.<id>.output}, ${variables.x}
// plus array indices and | default("...") filters.
// ---------------------------------------------------------------------------

/// Render every `${...}` occurrence in a JSON string.
fn render_template_string(template: &str, ctx: &ExecutionContext) -> String {
    let mut out = String::with_capacity(template.len());
    let bytes = template.as_bytes();
    let mut i = 0;
    while i < bytes.len() {
        if i + 1 < bytes.len() && bytes[i] == b'$' && bytes[i + 1] == b'{' {
            let start = i + 2;
            let mut end = start;
            while end < bytes.len() && bytes[end] != b'}' {
                end += 1;
            }
            if end < bytes.len() {
                let expr = &template[start..end];
                out.push_str(&render_expression(expr, ctx));
                i = end + 1;
            } else {
                out.push_str(&template[i..]);
                i = bytes.len();
            }
        } else {
            let c = template[i..].chars().next().expect("valid char boundary");
            out.push(c);
            i += c.len_utf8();
        }
    }
    out
}

fn render_expression(expr: &str, ctx: &ExecutionContext) -> String {
    // Support a single filter: | default("...")
    let (path_part, default_value) = if let Some(pipe_pos) = expr.find("|") {
        let path = expr[..pipe_pos].trim();
        let filter = expr[pipe_pos + 1..].trim();
        let default_value = parse_default_filter(filter);
        (path, default_value)
    } else {
        (expr.trim(), None)
    };

    let resolved = resolve_path(path_part, ctx);
    if resolved.is_empty() {
        default_value.unwrap_or_default()
    } else {
        resolved
    }
}

fn parse_default_filter(filter: &str) -> Option<String> {
    let filter = filter.trim();
    if !filter.starts_with("default(") || !filter.ends_with(")") {
        return None;
    }
    let inner = &filter[8..filter.len() - 1].trim();
    // Supports default("...") and default('...')
    if inner.len() >= 2 {
        let first = inner.chars().next().unwrap();
        let last = inner.chars().last().unwrap();
        if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
            return Some(inner[1..inner.len() - 1].to_string());
        }
    }
    None
}

fn render_value(value: &Value, ctx: &ExecutionContext) -> Value {
    match value {
        Value::String(s) => Value::String(render_template_string(s, ctx)),
        Value::Object(map) => {
            let mut out = serde_json::Map::new();
            for (k, v) in map {
                out.insert(k.clone(), render_value(v, ctx));
            }
            Value::Object(out)
        }
        Value::Array(arr) => Value::Array(arr.iter().map(|v| render_value(v, ctx)).collect()),
        other => other.clone(),
    }
}

fn resolve_path(path: &str, ctx: &ExecutionContext) -> String {
    if let Some(rest) = path.strip_prefix("trigger.") {
        json_path_to_string(&ctx.trigger, rest)
    } else if let Some(rest) = path.strip_prefix("steps.") {
        if let Some(step_id) = rest.strip_suffix(".output") {
            ctx.steps
                .get(step_id)
                .map(|r| r.output.clone())
                .unwrap_or_default()
        } else {
            String::new()
        }
    } else if let Some(name) = path.strip_prefix("variables.") {
        ctx.variables
            .get(name)
            .map(json_value_to_string)
            .unwrap_or_default()
    } else {
        String::new()
    }
}

fn json_path_to_string(value: &Value, path: &str) -> String {
    let mut current = value;
    for segment in path.split('.') {
        // Handle array indices like items[0]
        let (key, index) = if let Some(open) = segment.find('[') {
            let close = segment.find(']').unwrap_or(segment.len());
            let k = &segment[..open];
            let idx: Option<usize> = segment[open + 1..close].parse().ok();
            (k, idx)
        } else {
            (segment, None)
        };

        match current {
            Value::Object(map) => match map.get(key) {
                Some(v) => current = v,
                None => return String::new(),
            },
            _ => return String::new(),
        }

        if let Some(idx) = index {
            match current {
                Value::Array(arr) => {
                    current = arr.get(idx).unwrap_or(&Value::Null);
                }
                _ => return String::new(),
            }
        }
    }
    json_value_to_string(current)
}

fn json_value_to_string(v: &Value) -> String {
    match v {
        Value::String(s) => s.clone(),
        Value::Null => String::new(),
        other => other.to_string(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::RetryPolicy;
    use crate::domain::Workflow;
    use crate::noop::NoopAgent;
    use futures::StreamExt;
    use serde_json::json;

    fn single_step_workflow() -> Workflow {
        Workflow {
            entry: "s1".into(),
            steps: vec![Step {
                id: "s1".into(),
                agent: "a".into(),
                input: json!({"task": "hello"}),
                output: None,
                condition: None,
                next: None,
                on_error: None,
                timeout_seconds: None,
                retry: None,
                stream: true,
            }],
            variables: json!({}),
            error_policy: ErrorPolicy::default(),
        }
    }

    fn two_step_workflow() -> Workflow {
        Workflow {
            entry: "s1".into(),
            steps: vec![
                Step {
                    id: "s1".into(),
                    agent: "a".into(),
                    input: json!({"task": "first"}),
                    output: None,
                    condition: None,
                    next: None,
                    on_error: None,
                    timeout_seconds: None,
                    retry: None,
                    stream: true,
                },
                Step {
                    id: "s2".into(),
                    agent: "b".into(),
                    input: json!({"prev": "${steps.s1.output}"}),
                    output: None,
                    condition: None,
                    next: None,
                    on_error: None,
                    timeout_seconds: None,
                    retry: None,
                    stream: true,
                },
            ],
            variables: json!({}),
            error_policy: ErrorPolicy::default(),
        }
    }

    fn noop_agents() -> HashMap<String, Arc<dyn Agent>> {
        let mut m = HashMap::new();
        m.insert("a".into(), NoopAgent::arc());
        m.insert("b".into(), NoopAgent::arc());
        m
    }

    fn collect_chunks(stream: BoxStream<'static, Chunk>) -> Vec<Chunk> {
        let rt = tokio::runtime::Builder::new_current_thread()
            .enable_time()
            .build()
            .expect("tokio runtime");
        rt.block_on(async {
            let mut s = stream;
            let mut out = Vec::new();
            while let Some(c) = s.next().await {
                out.push(c);
            }
            out
        })
    }

    #[test]
    fn new_rejects_empty_workflow() {
        let wf = Workflow {
            entry: "s1".into(),
            steps: vec![],
            variables: json!({}),
            error_policy: ErrorPolicy::default(),
        };
        let err = match WorkflowEngine::new(wf, noop_agents()) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(
            matches!(err, Error::InvalidSpec(ref m) if m.contains("no steps")),
            "got: {err:?}"
        );
    }

    #[test]
    fn new_rejects_unknown_entry() {
        let mut wf = single_step_workflow();
        wf.entry = "ghost".into();
        let err = match WorkflowEngine::new(wf, noop_agents()) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(
            matches!(err, Error::InvalidSpec(ref m) if m.contains("workflow.entry")),
            "got: {err:?}"
        );
    }

    #[test]
    fn new_rejects_unknown_agent_reference() {
        let wf = Workflow {
            entry: "s1".into(),
            steps: vec![Step {
                id: "s1".into(),
                agent: "ghost".into(),
                input: json!({}),
                output: None,
                condition: None,
                next: None,
                on_error: None,
                timeout_seconds: None,
                retry: None,
                stream: true,
            }],
            variables: json!({}),
            error_policy: ErrorPolicy::default(),
        };
        let err = match WorkflowEngine::new(wf, noop_agents()) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(
            matches!(err, Error::InvalidSpec(ref m) if m.contains("unknown agent")),
            "got: {err:?}"
        );
    }

    #[test]
    fn new_rejects_cycle() {
        let wf = Workflow {
            entry: "s1".into(),
            steps: vec![
                Step {
                    id: "s1".into(),
                    agent: "a".into(),
                    input: json!({}),
                    output: None,
                    condition: None,
                    next: Some("s2".into()),
                    on_error: None,
                    timeout_seconds: None,
                    retry: None,
                    stream: true,
                },
                Step {
                    id: "s2".into(),
                    agent: "b".into(),
                    input: json!({}),
                    output: None,
                    condition: None,
                    next: Some("s1".into()),
                    on_error: None,
                    timeout_seconds: None,
                    retry: None,
                    stream: true,
                },
            ],
            variables: json!({}),
            error_policy: ErrorPolicy::default(),
        };
        let err = match WorkflowEngine::new(wf, noop_agents()) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(
            matches!(err, Error::InvalidSpec(ref m) if m.contains("cycle")),
            "got: {err:?}"
        );
    }

    #[test]
    fn single_step_emits_text_then_done() {
        let engine = WorkflowEngine::new(single_step_workflow(), noop_agents()).unwrap();
        let chunks = collect_chunks(engine.execute(json!({})));
        assert!(matches!(chunks.first(), Some(Chunk::Text(_))));
        assert!(matches!(chunks.last(), Some(Chunk::Done { .. })));
    }

    #[test]
    fn two_step_chains_output_through_template() {
        let engine = WorkflowEngine::new(two_step_workflow(), noop_agents()).unwrap();
        let chunks = collect_chunks(engine.execute(json!({})));
        let done = chunks.last().expect("should have done chunk");
        let done_vars = match done {
            Chunk::Done { variables } => variables.clone(),
            other => panic!("expected done, got: {other:?}"),
        };
        let s1 = done_vars
            .get("steps.s1.output")
            .and_then(|v| v.as_str())
            .expect("s1 output");
        let s2 = done_vars
            .get("steps.s2.output")
            .and_then(|v| v.as_str())
            .expect("s2 output");
        let s1_json = serde_json::to_string(s1).expect("serialize s1");
        assert!(
            s2.contains(&s1_json),
            "s2 input should embed JSON-escaped s1 output; s1={s1} s2={s2}"
        );
    }

    #[test]
    fn trigger_value_is_substituted() {
        let wf = Workflow {
            entry: "s1".into(),
            steps: vec![Step {
                id: "s1".into(),
                agent: "a".into(),
                input: json!({"msg": "${trigger.message}"}),
                output: None,
                condition: None,
                next: None,
                on_error: None,
                timeout_seconds: None,
                retry: None,
                stream: true,
            }],
            variables: json!({}),
            error_policy: ErrorPolicy::default(),
        };
        let engine = WorkflowEngine::new(wf, noop_agents()).unwrap();
        let chunks = collect_chunks(engine.execute(json!({"message": "hi there"})));
        let done = chunks.last().expect("done");
        let vars = match done {
            Chunk::Done { variables } => variables,
            _ => panic!(),
        };
        let s1 = vars
            .get("steps.s1.output")
            .and_then(|v| v.as_str())
            .unwrap();
        assert!(s1.contains("hi there"), "s1={s1}");
    }

    #[test]
    fn array_index_substitution() {
        let wf = Workflow {
            entry: "s1".into(),
            steps: vec![Step {
                id: "s1".into(),
                agent: "a".into(),
                input: json!({"msg": "${trigger.items[0].name}"}),
                output: None,
                condition: None,
                next: None,
                on_error: None,
                timeout_seconds: None,
                retry: None,
                stream: true,
            }],
            variables: json!({}),
            error_policy: ErrorPolicy::default(),
        };
        let engine = WorkflowEngine::new(wf, noop_agents()).unwrap();
        let chunks = collect_chunks(engine.execute(json!({"items": [{"name": "alpha"}]})));
        let done = chunks.last().expect("done");
        let vars = match done {
            Chunk::Done { variables } => variables,
            _ => panic!(),
        };
        let s1 = vars.get("steps.s1.output").and_then(|v| v.as_str()).unwrap();
        assert!(s1.contains("alpha"), "s1={s1}");
    }

    #[test]
    fn default_filter_substitution() {
        let wf = Workflow {
            entry: "s1".into(),
            steps: vec![Step {
                id: "s1".into(),
                agent: "a".into(),
                input: json!({"msg": "${trigger.missing | default(\"fallback\")}"}),
                output: None,
                condition: None,
                next: None,
                on_error: None,
                timeout_seconds: None,
                retry: None,
                stream: true,
            }],
            variables: json!({}),
            error_policy: ErrorPolicy::default(),
        };
        let engine = WorkflowEngine::new(wf, noop_agents()).unwrap();
        let chunks = collect_chunks(engine.execute(json!({})));
        let done = chunks.last().expect("done");
        let vars = match done {
            Chunk::Done { variables } => variables,
            _ => panic!(),
        };
        let s1 = vars.get("steps.s1.output").and_then(|v| v.as_str()).unwrap();
        assert!(s1.contains("fallback"), "s1={s1}");
    }

    #[test]
    fn condition_false_skips_step() {
        let wf = Workflow {
            entry: "s1".into(),
            steps: vec![
                Step {
                    id: "s1".into(),
                    agent: "a".into(),
                    input: json!({}),
                    output: None,
                    condition: Some("false".into()),
                    next: None,
                    on_error: None,
                    timeout_seconds: None,
                    retry: None,
                    stream: true,
                },
                Step {
                    id: "s2".into(),
                    agent: "b".into(),
                    input: json!({}),
                    output: None,
                    condition: None,
                    next: None,
                    on_error: None,
                    timeout_seconds: None,
                    retry: None,
                    stream: true,
                },
            ],
            variables: json!({}),
            error_policy: ErrorPolicy::default(),
        };
        let engine = WorkflowEngine::new(wf, noop_agents()).unwrap();
        let chunks = collect_chunks(engine.execute(json!({})));
        let done = chunks.last().expect("done");
        let vars = match done {
            Chunk::Done { variables } => variables,
            _ => panic!(),
        };
        assert!(
            vars.get("steps.s1.output").is_none(),
            "s1 should be skipped"
        );
        assert!(vars.get("steps.s2.output").is_some(), "s2 should run");
    }

    #[test]
    fn next_edge_jumps_over_step() {
        let wf = Workflow {
            entry: "s1".into(),
            steps: vec![
                Step {
                    id: "s1".into(),
                    agent: "a".into(),
                    input: json!({}),
                    output: None,
                    condition: None,
                    next: Some("s3".into()),
                    on_error: None,
                    timeout_seconds: None,
                    retry: None,
                    stream: true,
                },
                Step {
                    id: "s2".into(),
                    agent: "b".into(),
                    input: json!({}),
                    output: None,
                    condition: None,
                    next: None,
                    on_error: None,
                    timeout_seconds: None,
                    retry: None,
                    stream: true,
                },
                Step {
                    id: "s3".into(),
                    agent: "b".into(),
                    input: json!({}),
                    output: None,
                    condition: None,
                    next: None,
                    on_error: None,
                    timeout_seconds: None,
                    retry: None,
                    stream: true,
                },
            ],
            variables: json!({}),
            error_policy: ErrorPolicy::default(),
        };
        let engine = WorkflowEngine::new(wf, noop_agents()).unwrap();
        let chunks = collect_chunks(engine.execute(json!({})));
        let done = chunks.last().expect("done");
        let vars = match done {
            Chunk::Done { variables } => variables,
            _ => panic!(),
        };
        assert!(vars.get("steps.s1.output").is_some());
        assert!(vars.get("steps.s2.output").is_none(), "s2 should be skipped");
        assert!(vars.get("steps.s3.output").is_some());
    }

    #[test]
    fn on_error_jumps_to_recovery_step() {
        use crate::agent::{AgentSchema, HealthStatus};
        use async_trait::async_trait;

        struct FailingAgent;

        #[async_trait]
        impl Agent for FailingAgent {
            async fn invoke(
                &self,
                _input: Value,
                _ctx: InvokeContext,
            ) -> Result<BoxStream<'static, Chunk>> {
                Err(Error::Adapter("boom".into()))
            }
            async fn health(&self) -> HealthStatus {
                HealthStatus {
                    healthy: true,
                    message: None,
                }
            }
            async fn schema(&self) -> AgentSchema {
                AgentSchema {
                    name: "failing".into(),
                    input_schema: json!({"type": "object"}),
                    output_schema: json!({"type": "string"}),
                }
            }
        }

        let wf = Workflow {
            entry: "s1".into(),
            steps: vec![
                Step {
                    id: "s1".into(),
                    agent: "failing".into(),
                    input: json!({}),
                    output: None,
                    condition: None,
                    next: None,
                    on_error: Some("s2".into()),
                    timeout_seconds: None,
                    retry: None,
                    stream: true,
                },
                Step {
                    id: "s2".into(),
                    agent: "a".into(),
                    input: json!({"recovered": true}),
                    output: None,
                    condition: None,
                    next: None,
                    on_error: None,
                    timeout_seconds: None,
                    retry: None,
                    stream: true,
                },
            ],
            variables: json!({}),
            error_policy: ErrorPolicy::default(),
        };
        let mut agents = noop_agents();
        agents.insert("failing".into(), Arc::new(FailingAgent));
        let engine = WorkflowEngine::new(wf, agents).unwrap();
        let chunks = collect_chunks(engine.execute(json!({})));
        let done = chunks.last().expect("done");
        let vars = match done {
            Chunk::Done { variables } => variables,
            _ => panic!(),
        };
        assert!(
            vars.get("steps.s1.output").is_some(),
            "s1 should have an error output"
        );
        assert!(
            vars.get("steps.s2.output").is_some(),
            "s2 recovery should run"
        );
    }

    #[test]
    fn retry_then_success() {
        use crate::agent::{AgentSchema, HealthStatus};
        use async_trait::async_trait;
        use std::sync::atomic::{AtomicUsize, Ordering};

        static CALLS: AtomicUsize = AtomicUsize::new(0);

        struct SometimesAgent;

        #[async_trait]
        impl Agent for SometimesAgent {
            async fn invoke(
                &self,
                input: Value,
                _ctx: InvokeContext,
            ) -> Result<BoxStream<'static, Chunk>> {
                let n = CALLS.fetch_add(1, Ordering::SeqCst);
                if n == 0 {
                    return Err(Error::Adapter("first call fails".into()));
                }
                Ok(crate::noop::NoopAgent::arc().invoke(input, _ctx).await?)
            }
            async fn health(&self) -> HealthStatus {
                HealthStatus {
                    healthy: true,
                    message: None,
                }
            }
            async fn schema(&self) -> AgentSchema {
                AgentSchema {
                    name: "sometimes".into(),
                    input_schema: json!({"type": "object"}),
                    output_schema: json!({"type": "string"}),
                }
            }
        }

        let wf = Workflow {
            entry: "s1".into(),
            steps: vec![Step {
                id: "s1".into(),
                agent: "sometimes".into(),
                input: json!({}),
                output: None,
                condition: None,
                next: None,
                on_error: None,
                timeout_seconds: None,
                retry: Some(RetryPolicy {
                    count: 2,
                    backoff_ms: 1,
                }),
                stream: true,
            }],
            variables: json!({}),
            error_policy: ErrorPolicy::default(),
        };
        CALLS.store(0, Ordering::SeqCst);
        let mut agents = noop_agents();
        agents.insert("sometimes".into(), Arc::new(SometimesAgent));
        let engine = WorkflowEngine::new(wf, agents).unwrap();
        let chunks = collect_chunks(engine.execute(json!({})));
        assert!(matches!(chunks.last(), Some(Chunk::Done { .. })));
        assert_eq!(CALLS.load(Ordering::SeqCst), 2);
    }
}
