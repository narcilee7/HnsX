//! Agent trait, AgentSpec, and the spec types nested under it.

use async_trait::async_trait;
use futures::stream::BoxStream;
use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::chunk::Chunk;
use crate::error::Result;
use crate::sandbox::SandboxSpec;

/// Per-invocation context passed to an Agent's `invoke` method.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct InvokeContext {
    pub session_id: String,
    pub domain_id: String,
    pub agent_id: String,
}

/// Input / output schema description for an agent.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentSchema {
    pub name: String,
    pub input_schema: Value,
    pub output_schema: Value,
}

/// Health snapshot reported by an Agent or Adapter.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HealthStatus {
    pub healthy: bool,
    #[serde(default)]
    pub message: Option<String>,
}

#[async_trait]
pub trait Agent: Send + Sync {
    async fn invoke(&self, input: Value, ctx: InvokeContext) -> Result<BoxStream<'static, Chunk>>;
    async fn health(&self) -> HealthStatus;
    async fn schema(&self) -> AgentSchema;
}

// ---- Spec types (parsed from domain.yaml) ----

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentSpec {
    pub id: String,
    pub description: String,
    pub model: ModelRef,
    pub adapter: AdapterConfig,
    #[serde(default)]
    pub tools: Vec<ToolRef>,
    pub prompt: PromptTemplate,
    #[serde(default)]
    pub sandbox: Option<SandboxSpec>,
    /// Number of recent turns this agent keeps in memory context.
    #[serde(default)]
    pub memory_window: Option<usize>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ModelRef {
    pub provider: Provider,
    pub model: String,
    #[serde(default)]
    pub endpoint: Option<String>,
}

/// Supported model providers. Serialized as `kebab-case` in YAML.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "kebab-case")]
pub enum Provider {
    Openai,
    Anthropic,
    ClaudeCode,
    Codex,
    Ollama,
    Custom,
}

/// Adapter-level config (timeouts, custom fields). Specified per agent.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AdapterConfig {
    #[serde(default)]
    pub timeout_seconds: Option<u64>,
    #[serde(default)]
    pub extra: Value,
}

/// Retry policy for a single step or agent invocation.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
pub struct RetryPolicy {
    #[serde(default = "default_retry_count")]
    pub count: u32,
    #[serde(default = "default_retry_backoff_ms")]
    pub backoff_ms: u64,
}

impl Default for RetryPolicy {
    fn default() -> Self {
        Self {
            count: default_retry_count(),
            backoff_ms: default_retry_backoff_ms(),
        }
    }
}

fn default_retry_count() -> u32 {
    0
}

fn default_retry_backoff_ms() -> u64 {
    1000
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolRef {
    pub kind: ToolKind,
    pub name: String,
    #[serde(default)]
    pub config: Value,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq, Hash)]
#[serde(rename_all = "lowercase")]
pub enum ToolKind {
    Http,
    Python,
    Sql,
    Shell,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PromptTemplate {
    pub template: String,
    #[serde(default)]
    pub variables: Value,
}
