//! Adapter trait wrapping external AI agent providers.

use async_trait::async_trait;
use futures::stream::BoxStream;
use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::agent::HealthStatus;
use crate::chunk::Chunk;
use crate::error::Result;

/// Per-invocation runtime context provided to an Adapter.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RuntimeContext {
    pub session_id: String,
    #[serde(default)]
    pub config: Value,
}

#[async_trait]
pub trait Adapter: Send + Sync {
    async fn prepare(&self, config: &Value) -> Result<RuntimeContext>;
    async fn invoke(
        &self,
        input: &Value,
        ctx: &RuntimeContext,
    ) -> Result<BoxStream<'static, Chunk>>;
    async fn teardown(&self, ctx: &RuntimeContext) -> Result<()>;
    async fn health(&self) -> HealthStatus;
}
