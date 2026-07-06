//! `DomainLoader`: parses a [`DomainSpec`] YAML and returns an `Arc<dyn Domain>`.
//!
//! Phase 1.1 — constructs the domain but does not execute it. Execution lands
//! in Phase 1.2 (workflow engine) and adapter wiring in Phase 1.4+. The
//! returned [`Domain`] impl surfaces the spec and validation results, while
//! `invoke` and `get_agent` return `Error::Unimplemented` until later phases
//! fill in those pieces.

use std::collections::{HashMap, HashSet};
use std::fs::File;
use std::path::Path;
use std::sync::Arc;

use async_trait::async_trait;
use futures::stream::BoxStream;
use serde_json::Value;

use crate::agent::{Agent, AgentSpec};
use crate::chunk::Chunk;
use crate::domain::{Domain, DomainSpec};
use crate::error::{Error, Result};
use crate::noop::NoopAgent;
use crate::workflow::WorkflowEngine;

/// A loaded (parsed + validated) domain.
#[derive(Clone)]
pub struct LoadedDomain {
    spec: DomainSpec,
    /// Agents indexed by id. The full `Arc<dyn Agent>` impls are constructed
    /// once the adapter factory exists (Phase 1.4+).
    agents: HashMap<String, AgentSpec>,
    /// Pre-built workflow engine, holding one noop agent per spec agent.
    /// Lands in Phase 1.2 so `Domain::invoke` can run end-to-end against
    /// stubbed agents; real adapters swap in at Phase 1.4+.
    engine: WorkflowEngine,
}

impl LoadedDomain {
    /// Build a `LoadedDomain` from a parsed spec, validating it first.
    pub fn new(spec: DomainSpec) -> Result<Self> {
        let agents = validate(&spec)?;
        let engine = build_engine(&spec, &agents)?;
        Ok(Self {
            spec,
            agents,
            engine,
        })
    }

    /// Borrow the agent spec for the given id, if any.
    pub fn agent_spec(&self, id: &str) -> Option<&AgentSpec> {
        self.agents.get(id)
    }

    /// All agent specs indexed by id.
    pub fn agent_specs(&self) -> &HashMap<String, AgentSpec> {
        &self.agents
    }
}

#[async_trait]
impl Domain for LoadedDomain {
    async fn invoke(&self, trigger: Value) -> Result<BoxStream<'static, Chunk>> {
        // Each chunk is flattened into the stream, errors short-circuit the run.
        let inner = self.engine.execute(trigger);
        Ok(Box::pin(inner))
    }

    fn get_agent(&self, id: &str) -> Option<Arc<dyn Agent>> {
        // Real Agent impls (wrapping an Adapter + Sandbox) are constructed by
        // the loader once `hnsx-adapter` and `hnsx-sandbox` ship their first
        // implementations. For 1.2 the engine uses noop agents internally;
        // we do not expose them via this method yet.
        let _ = id;
        None
    }

    fn spec(&self) -> &DomainSpec {
        &self.spec
    }

    fn agent_spec(&self, id: &str) -> Option<&AgentSpec> {
        self.agents.get(id)
    }
}

/// Stateless YAML → `Arc<dyn Domain>` loader.
///
/// In later phases this will hold an `AdapterFactory` and a `SandboxFactory`
/// so it can materialize real `Agent` instances.
#[derive(Debug, Default, Clone, Copy)]
pub struct DomainLoader;

impl DomainLoader {
    pub fn new() -> Self {
        Self
    }

    /// Load a domain from a YAML file on disk.
    pub fn from_path(&self, path: impl AsRef<Path>) -> Result<Arc<dyn Domain>> {
        let file = File::open(path.as_ref())?;
        let spec: DomainSpec = serde_yaml::from_reader(file)?;
        self.finish(spec)
    }

    /// Load a domain from an in-memory YAML string. Useful for tests.
    pub fn from_str(&self, yaml: &str) -> Result<Arc<dyn Domain>> {
        let spec: DomainSpec = serde_yaml::from_str(yaml)?;
        self.finish(spec)
    }

    fn finish(&self, spec: DomainSpec) -> Result<Arc<dyn Domain>> {
        let domain = LoadedDomain::new(spec)?;
        Ok(Arc::new(domain))
    }
}

/// Structural validation of a [`DomainSpec`].
///
/// Returns the agent map (id → spec) on success. On failure, returns an
/// `Error::InvalidSpec` with a human-readable message.
///
/// Scope for Phase 1.1: id uniqueness + cross-references. Cycle detection and
/// input/output schema compatibility land with the workflow engine in 1.2.
fn validate(spec: &DomainSpec) -> Result<HashMap<String, AgentSpec>> {
    // 1. No duplicate agent ids.
    let mut agents: HashMap<String, AgentSpec> = HashMap::with_capacity(spec.agents.len());
    for agent in &spec.agents {
        if agents.insert(agent.id.clone(), agent.clone()).is_some() {
            return Err(Error::InvalidSpec(format!(
                "duplicate agent id: {}",
                agent.id
            )));
        }
    }

    // 2. No duplicate step ids, and remember the set for cross-references.
    let mut step_ids: HashSet<&str> = HashSet::with_capacity(spec.workflow.steps.len());
    for step in &spec.workflow.steps {
        if !step_ids.insert(step.id.as_str()) {
            return Err(Error::InvalidSpec(format!(
                "duplicate step id: {}",
                step.id
            )));
        }
    }

    // 3. workflow.entry must match a step id.
    if !step_ids.contains(spec.workflow.entry.as_str()) {
        return Err(Error::InvalidSpec(format!(
            "workflow.entry '{}' does not match any step id",
            spec.workflow.entry
        )));
    }

    // 4. Each step's `agent` must reference an existing agent.
    for step in &spec.workflow.steps {
        if !agents.contains_key(&step.agent) {
            return Err(Error::InvalidSpec(format!(
                "step '{}' references unknown agent '{}'",
                step.id, step.agent
            )));
        }
    }

    Ok(agents)
}

/// Build a workflow engine by wiring one noop agent per spec agent.
///
/// Phase 1.2 keeps this simple — every spec agent maps to the same noop.
/// Phase 1.4+ will replace this with an `AdapterFactory`-driven dispatch.
fn build_engine(spec: &DomainSpec, agents: &HashMap<String, AgentSpec>) -> Result<WorkflowEngine> {
    let mut engine_agents: HashMap<String, Arc<dyn Agent>> = HashMap::with_capacity(agents.len());
    for id in agents.keys() {
        engine_agents.insert(id.clone(), NoopAgent::arc());
    }
    WorkflowEngine::new(spec.workflow.clone(), engine_agents)
}

#[cfg(test)]
mod tests {
    use super::*;

    const VALID_YAML: &str = r#"
id: test
version: 0.1.0
description: A tiny test domain.
agents:
  - id: a
    description: agent a
    model: { provider: openai, model: gpt-4o-mini }
    adapter: { timeout_seconds: 30 }
    prompt:
      template: "hello"
      variables: {}
workflow:
  entry: s1
  steps:
    - id: s1
      agent: a
      output: out
"#;

    fn loader() -> DomainLoader {
        DomainLoader::new()
    }

    #[test]
    fn loads_valid_yaml() {
        let domain = loader().from_str(VALID_YAML).expect("should load");
        assert_eq!(domain.spec().id, "test");
        assert_eq!(domain.spec().agents.len(), 1);
        assert_eq!(domain.spec().workflow.steps.len(), 1);
    }

    #[test]
    fn rejects_malformed_yaml() {
        let bad = "id: : :";
        let err = match loader().from_str(bad) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        // Either yaml parse error or serde validation error is acceptable;
        // the point is that it does not silently succeed.
        assert!(
            matches!(err, Error::Yaml(_) | Error::InvalidSpec(_)),
            "got: {err:?}"
        );
    }

    #[test]
    fn rejects_missing_required_fields() {
        // Missing `workflow`.
        let bad = r#"
id: test
version: 0.1.0
description: x
agents: []
"#;
        let err = match loader().from_str(bad) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(matches!(err, Error::Yaml(_)), "got: {err:?}");
    }

    #[test]
    fn rejects_duplicate_agent_id() {
        let bad = r#"
id: test
version: 0.1.0
description: x
agents:
  - { id: a, description: x, model: { provider: openai, model: x }, adapter: {}, prompt: { template: t, variables: {} } }
  - { id: a, description: y, model: { provider: openai, model: x }, adapter: {}, prompt: { template: t, variables: {} } }
workflow:
  entry: s1
  steps:
    - { id: s1, agent: a }
"#;
        let err = match loader().from_str(bad) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(
            matches!(err, Error::InvalidSpec(ref m) if m.contains("duplicate agent")),
            "got: {err:?}"
        );
    }

    #[test]
    fn rejects_duplicate_step_id() {
        let bad = r#"
id: test
version: 0.1.0
description: x
agents:
  - { id: a, description: x, model: { provider: openai, model: x }, adapter: {}, prompt: { template: t, variables: {} } }
  - { id: b, description: y, model: { provider: openai, model: x }, adapter: {}, prompt: { template: t, variables: {} } }
workflow:
  entry: s1
  steps:
    - { id: s1, agent: a }
    - { id: s1, agent: b }
"#;
        let err = match loader().from_str(bad) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(
            matches!(err, Error::InvalidSpec(ref m) if m.contains("duplicate step")),
            "got: {err:?}"
        );
    }

    #[test]
    fn rejects_unknown_agent_reference() {
        let bad = r#"
id: test
version: 0.1.0
description: x
agents:
  - { id: a, description: x, model: { provider: openai, model: x }, adapter: {}, prompt: { template: t, variables: {} } }
workflow:
  entry: s1
  steps:
    - { id: s1, agent: ghost }
"#;
        let err = match loader().from_str(bad) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(
            matches!(err, Error::InvalidSpec(ref m) if m.contains("unknown agent")),
            "got: {err:?}"
        );
    }

    #[test]
    fn rejects_unknown_workflow_entry() {
        let bad = r#"
id: test
version: 0.1.0
description: x
agents:
  - { id: a, description: x, model: { provider: openai, model: x }, adapter: {}, prompt: { template: t, variables: {} } }
workflow:
  entry: s_ghost
  steps:
    - { id: s1, agent: a }
"#;
        let err = match loader().from_str(bad) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(
            matches!(err, Error::InvalidSpec(ref m) if m.contains("workflow.entry")),
            "got: {err:?}"
        );
    }

    #[test]
    fn from_path_missing_file_returns_io_error() {
        let err = match loader().from_path("/nonexistent/path.yaml") {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(matches!(err, Error::Io(_)), "got: {err:?}");
    }

    #[test]
    fn invoke_streams_noop_output() {
        use futures::StreamExt;
        // Phase 1.2: Domain::invoke runs the workflow against noop agents
        // and yields at least one Chunk::text plus a final Chunk::done.
        let domain = loader().from_str(VALID_YAML).expect("should load");
        let mut stream = futures::executor::block_on(domain.invoke(serde_json::json!({})))
            .expect("invoke should succeed");

        let mut saw_text = false;
        let mut saw_done = false;
        while let Some(chunk) = futures::executor::block_on(stream.next()) {
            match chunk {
                Chunk::Text(_) => saw_text = true,
                Chunk::Done { .. } => saw_done = true,
                _ => {}
            }
        }
        assert!(saw_text, "expected at least one text chunk");
        assert!(saw_done, "expected a final done chunk");
    }

    #[test]
    fn get_agent_returns_none_until_adapters_land() {
        let domain = loader().from_str(VALID_YAML).expect("should load");
        assert!(domain.get_agent("a").is_none());
        // But the spec lookup works.
        let spec = domain.agent_spec("a").expect("spec should exist");
        assert_eq!(spec.id, "a");
    }
}
