//! Domain trait and `DomainSpec` — the top-level unit of business definition.

use std::sync::Arc;

use async_trait::async_trait;
use futures::stream::BoxStream;
use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::agent::{Agent, AgentSpec};
use crate::chunk::Chunk;
use crate::error::Result;
use crate::memory::MemoryConfig;
use crate::sandbox::SandboxPolicy;

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
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Step {
    pub id: String,
    pub agent: String,
    #[serde(default)]
    pub input: Value,
    #[serde(default)]
    pub output: Option<String>,
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
