//! Domain trait and `DomainSpec` — the top-level unit of business definition.

use std::sync::Arc;

use async_trait::async_trait;
use futures::stream::BoxStream;
use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::agent::{Agent, AgentSpec, RetryPolicy};
use crate::chunk::Chunk;
use crate::error::Result;
use crate::memory::MemoryConfig;
use crate::sandbox::SandboxPolicy;

/// How the workflow should behave when a step fails after all retries.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
#[serde(rename_all = "snake_case")]
pub enum ErrorPolicy {
    /// Stop the workflow immediately (default).
    #[default]
    FailFast,
    /// Record the failure and continue with the next step.
    Continue,
    /// Jump to the named fallback step.
    FallbackStep(String),
}

/// A complete domain definition, loaded from YAML.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DomainSpec {
    pub id: String,
    pub version: String,
    pub description: String,
    pub agents: Vec<AgentSpec>,
    pub workflow: Workflow,
    #[serde(default)]
    pub memory: Option<MemoryConfig>,
    #[serde(default)]
    pub sandbox: Option<SandboxPolicy>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Workflow {
    pub entry: String,
    pub steps: Vec<Step>,
    #[serde(default)]
    pub variables: Value,
    /// How the workflow behaves when a step fails after retries.
    #[serde(default)]
    pub error_policy: ErrorPolicy,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Step {
    pub id: String,
    pub agent: String,
    #[serde(default)]
    pub input: Value,
    #[serde(default)]
    pub output: Option<String>,
    /// Optional template string. The step runs iff the rendered value is
    /// non-empty and not the literal `"false"`. Supports the same
    /// `${...}` substitution as `input`.
    #[serde(default)]
    pub condition: Option<String>,
    /// Step id to jump to after a successful execution.
    /// If omitted, execution continues with the next step in YAML order.
    #[serde(default)]
    pub next: Option<String>,
    /// Step id to jump to after execution fails (after retries are exhausted).
    #[serde(default)]
    pub on_error: Option<String>,
    /// Maximum time the step is allowed to run, in seconds.
    #[serde(default)]
    pub timeout_seconds: Option<u64>,
    /// Retry policy for this step.
    #[serde(default)]
    pub retry: Option<RetryPolicy>,
    /// Whether to stream step chunks in real time. Defaults to true.
    #[serde(default = "default_stream")]
    pub stream: bool,
}

fn default_stream() -> bool {
    true
}

#[async_trait]
pub trait Domain: Send + Sync {
    async fn invoke(&self, trigger: Value) -> Result<BoxStream<'static, Chunk>>;
    fn get_agent(&self, id: &str) -> Option<Arc<dyn Agent>>;
    fn spec(&self) -> &DomainSpec;

    /// Look up an agent spec by id without constructing an `Arc<dyn Agent>`.
    /// Useful for tooling (validation, documentation) before adapter wiring lands.
    fn agent_spec(&self, id: &str) -> Option<&AgentSpec> {
        let _ = id;
        None
    }
}
