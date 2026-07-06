//! `WorkflowEngine`: executes a `Workflow` over a set of agents, yielding
//! a stream of `Chunk`s.
//!
//! Phase 1.2 scope:
//! - Linear execution (steps in YAML order).
//! - `output` binding: each step's aggregated text is exposed as
//!   `steps.<step_id>.output`.
//! - Input rendering: recursive `${...}` substitution for
//!   `trigger.<path>`, `steps.<id>.output`, and `variables.<name>`.
//! - `condition`: optional template; step runs iff the rendered value is
//!   non-empty and not the literal `"false"`.
//!
//! `petgraph` is used to back the structure (so cycle detection and
//! branching in 1.2.1+ are a small addition), but for 1.2 the graph is
//! implicit-linear and the execution order matches the input order.

use std::collections::HashMap;
use std::sync::Arc;

use async_stream::stream;
use futures::stream::{BoxStream, StreamExt};
use petgraph::algo::toposort;
use petgraph::graph::{DiGraph, NodeIndex};
use serde_json::Value;
use uuid::Uuid;

use crate::agent::{Agent, InvokeContext};
use crate::chunk::Chunk;
use crate::domain::{Step, Workflow};
use crate::error::{Error, Result};
use crate::telemetry::{StepTrace, Telemetry, now_ms};

/// Outcome of a single step: concatenated text + all chunks received.
#[derive(Debug, Default, Clone)]
pub struct StepResult {
    /// Concatenation of every `Chunk::Text` yielded by the agent.
    pub output: String,
    /// Full chunk history (for debugging / replay).
    pub chunks: Vec<Chunk>,
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
    /// Same steps, indexed by id for O(1) lookup and as a sanity check
    /// against duplicates (the loader already rejects those).
    step_index: HashMap<String, usize>,
    entry: String,
    /// Optional telemetry sink. When present, every completed step is
    /// recorded as one JSONL line under `$session_id.jsonl`. 1.7.
    telemetry: Option<Arc<Telemetry>>,
    /// Domain id attached to every StepTrace produced by this engine.
    /// 1.7.
    domain_id: String,
}

impl WorkflowEngine {
    /// Build an engine from a parsed `Workflow` and an agent map.
    ///
    /// Validates that the workflow has at least one step, that `entry`
    /// references a known step, and that the implied graph is acyclic
    /// (linear, so trivially true for 1.2 but enforced via petgraph).
    pub fn new(workflow: Workflow, agents: HashMap<String, Arc<dyn Agent>>) -> Result<Self> {
        Self::new_with_telemetry(workflow, agents, None, String::new())
    }

    /// Build an engine that also writes per-step traces to `telemetry`.
    /// `domain_id` is attached to every record.
    pub fn new_with_telemetry(
        workflow: Workflow,
        agents: HashMap<String, Arc<dyn Agent>>,
        telemetry: Option<Arc<Telemetry>>,
        domain_id: String,
    ) -> Result<Self> {
        if workflow.steps.is_empty() {
            return Err(Error::InvalidSpec("workflow has no steps".into()));
        }

        // Build step index, preserving YAML order as the execution order.
        let mut step_index: HashMap<String, usize> = HashMap::with_capacity(workflow.steps.len());
        for (i, step) in workflow.steps.iter().enumerate() {
            step_index.insert(step.id.clone(), i);
        }

        if !step_index.contains_key(&workflow.entry) {
            return Err(Error::InvalidSpec(format!(
                "workflow.entry '{}' does not match any step id",
                workflow.entry
            )));
        }

        // Build the petgraph and assert no cycles. For 1.2 the graph has
        // no edges (linear); topo sort just returns the insertion order.
        let mut graph: DiGraph<String, ()> = DiGraph::new();
        let mut nodes: HashMap<&str, NodeIndex> = HashMap::new();
        for step in &workflow.steps {
            let idx = graph.add_node(step.id.clone());
            nodes.insert(step.id.as_str(), idx);
        }
        if toposort(&graph, None).is_err() {
            return Err(Error::InvalidSpec("workflow graph has a cycle".into()));
        }

        Ok(Self {
            steps: workflow.steps,
            agents,
            step_index,
            entry: workflow.entry,
            telemetry,
            domain_id,
        })
    }

    /// Execute the workflow against `trigger`, returning a chunk stream.
    pub fn execute(&self, trigger: Value) -> BoxStream<'static, Chunk> {
        let engine = self.clone();
        // Single session id per execution, attached to every step trace.
        let session_id = Uuid::new_v4().to_string();
        Box::pin(stream! {
            let mut ctx = ExecutionContext::new(trigger, engine.workflow_variables());
            let mut current_index = engine.step_index[&engine.entry];

            loop {
                if current_index >= engine.steps.len() {
                    break;
                }
                let step = engine.steps[current_index].clone();

                // 1. Condition gate.
                if let Some(cond) = &step.condition {
                    let rendered = render_template_string(cond, &ctx);
                    let run = !rendered.is_empty() && rendered != "false";
                    if !run {
                        current_index += 1;
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
                let input = render_value(&step.input, &ctx);

                // 4. Invoke agent and stream chunks.
                let invoke_ctx = InvokeContext {
                    session_id: session_id.clone(),
                    domain_id: engine.domain_id.clone(),
                    agent_id: step.agent.clone(),
                };

                let started_at_ms = now_ms();
                let mut stream = match agent.invoke(input.clone(), invoke_ctx).await {
                    Ok(s) => s,
                    Err(e) => {
                        yield Chunk::error(format!("step '{}' invoke failed: {e}", step.id));
                        return;
                    }
                };

                let mut result = StepResult::default();
                while let Some(chunk) = stream.next().await {
                    if let Chunk::Text(t) = &chunk {
                        if crate::pii::contains_pii(t) {
                            yield Chunk::error("PII detected in agent output; stream halted".to_string());
                            return;
                        }
                        result.output.push_str(t);
                    }
                    result.chunks.push(chunk.clone());
                    yield chunk;
                }

                // 5. Emit telemetry (1.7): one record per completed step.
                if let Some(tel) = engine.telemetry.as_ref() {
                    let trace = StepTrace {
                        session_id: session_id.clone(),
                        domain_id: engine.domain_id.clone(),
                        step_id: step.id.clone(),
                        agent_id: step.agent.clone(),
                        started_at_ms,
                        duration_ms: now_ms().saturating_sub(started_at_ms),
                        input: input.clone(),
                        output: result.output.clone(),
                    };
                    if let Err(e) = tel.record_step(&trace) {
                        yield Chunk::error(format!("telemetry record failed: {e}"));
                        return;
                    }
                }

                ctx.steps.insert(step.id.clone(), result);
                current_index += 1;
            }

            // Final Chunk::done carries the step outputs as a flat object.
            let mut done_vars = serde_json::Map::new();
            for (k, v) in &ctx.steps {
                done_vars.insert(format!("steps.{k}.output"), Value::String(v.output.clone()));
            }
            yield Chunk::done(Value::Object(done_vars));
        })
    }

    /// Workflow-level variables. Exposed as `${variables.<name>}` in templates.
    /// For 1.2 the variables are not in `Workflow` itself (they are still
    /// on the spec); we read from each step's already-rendered context,
    /// not from the workflow spec, so this is a no-op for now.
    fn workflow_variables(&self) -> HashMap<String, Value> {
        HashMap::new()
    }
}

// ---------------------------------------------------------------------------
// Template rendering: ${trigger.x.y}, ${steps.<id>.output}, ${variables.x}
// ---------------------------------------------------------------------------

/// Render every `${...}` occurrence in a JSON string.
fn render_template_string(template: &str, ctx: &ExecutionContext) -> String {
    let mut out = String::with_capacity(template.len());
    let bytes = template.as_bytes();
    let mut i = 0;
    while i < bytes.len() {
        if i + 1 < bytes.len() && bytes[i] == b'$' && bytes[i + 1] == b'{' {
            // Find matching `}` (no nesting in 1.2).
            let start = i + 2;
            let mut end = start;
            while end < bytes.len() && bytes[end] != b'}' {
                end += 1;
            }
            if end < bytes.len() {
                let path = &template[start..end];
                out.push_str(&resolve_path(path, ctx));
                i = end + 1;
            } else {
                // Unterminated `${` — keep as literal.
                out.push_str(&template[i..]);
                i = bytes.len();
            }
        } else {
            // Push the next char (could be multi-byte; push the char).
            let c = template[i..].chars().next().expect("valid char boundary");
            out.push(c);
            i += c.len_utf8();
        }
    }
    out
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
        // Expected form: `<step_id>.output`
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
        match current {
            Value::Object(map) => match map.get(segment) {
                Some(v) => current = v,
                None => return String::new(),
            },
            _ => return String::new(),
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
            }],
            variables: json!({}),
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
                },
                Step {
                    id: "s2".into(),
                    agent: "b".into(),
                    input: json!({"prev": "${steps.s1.output}"}),
                    output: None,
                    condition: None,
                },
            ],
            variables: json!({}),
        }
    }

    fn noop_agents() -> HashMap<String, Arc<dyn Agent>> {
        let mut m = HashMap::new();
        m.insert("a".into(), NoopAgent::arc());
        m.insert("b".into(), NoopAgent::arc());
        m
    }

    fn collect_chunks(stream: BoxStream<'static, Chunk>) -> Vec<Chunk> {
        futures::executor::block_on(async {
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
        // Last chunk is Chunk::done with the variables map.
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
        // s2's input was `${steps.s1.output}`, so the rendered value is the
        // JSON-serialized form of s1. s2's noop echoes that as a string.
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
            }],
            variables: json!({}),
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
                },
                Step {
                    id: "s2".into(),
                    agent: "b".into(),
                    input: json!({}),
                    output: None,
                    condition: None,
                },
            ],
            variables: json!({}),
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
    fn condition_true_runs_step() {
        let wf = Workflow {
            entry: "s1".into(),
            steps: vec![Step {
                id: "s1".into(),
                agent: "a".into(),
                input: json!({}),
                output: None,
                condition: Some("true".into()),
            }],
            variables: json!({}),
        };
        let engine = WorkflowEngine::new(wf, noop_agents()).unwrap();
        let chunks = collect_chunks(engine.execute(json!({})));
        let done = chunks.last().expect("done");
        let vars = match done {
            Chunk::Done { variables } => variables,
            _ => panic!(),
        };
        assert!(vars.get("steps.s1.output").is_some());
    }
}
