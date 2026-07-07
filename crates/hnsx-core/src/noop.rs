//! `NoopAgent`: deterministic agent for end-to-end testing.
//!
//! Echoes the input it received as a single `Chunk::text` so the workflow
//! engine can be exercised without a real adapter. Lands in Phase 1.2
//! alongside the workflow engine; real provider adapters replace it in
//! Phase 1.4+.

use std::sync::Arc;

use async_trait::async_trait;
use futures::stream::{self, BoxStream};
use serde_json::{Value, json};

use crate::agent::{Agent, AgentSchema, HealthStatus, InvokeContext};
use crate::chunk::Chunk;
use crate::error::Result;

/// A trivial agent that yields one `Chunk::text` containing the JSON-encoded
/// input it received, prefixed with `noop:`.
pub struct NoopAgent;

impl NoopAgent {
    pub fn arc() -> Arc<dyn Agent> {
        Arc::new(Self)
    }
}

#[async_trait]
impl Agent for NoopAgent {
    #[tracing::instrument(skip(self, _ctx), level = "debug")]
    async fn invoke(&self, input: Value, _ctx: InvokeContext) -> Result<BoxStream<'static, Chunk>> {
        let text = format!("noop: {}", input);
        let stream = stream::once(async move { Chunk::text(text) });
        Ok(Box::pin(stream))
    }

    async fn health(&self) -> HealthStatus {
        HealthStatus {
            healthy: true,
            message: None,
        }
    }

    async fn schema(&self) -> AgentSchema {
        AgentSchema {
            name: "noop".to_string(),
            input_schema: json!({"type": "object"}),
            output_schema: json!({"type": "string"}),
        }
    }
}
