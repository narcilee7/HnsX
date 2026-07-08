//! Native custom / OpenAI-compatible API adapter.
//!
//! Talks to any OpenAI-compatible chat completions endpoint. The API key is
//! read from `CUSTOM_API_KEY`; the base URL must be provided via
//! `spec.model.endpoint`.

use async_trait::async_trait;
use futures::stream::BoxStream;
use serde_json::Value;

use crate::openai_adapter::OpenAIAdapter;
use hnsx_core::adapter::{Adapter, RuntimeContext};
use hnsx_core::agent::{AgentSpec, HealthStatus};
use hnsx_core::chunk::Chunk;
use hnsx_core::error::{Error, Result};
use hnsx_core::tool::ToolRegistry;

pub struct CustomAdapter {
    inner: OpenAIAdapter,
}

impl std::fmt::Debug for CustomAdapter {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("CustomAdapter").finish_non_exhaustive()
    }
}

impl CustomAdapter {
    pub fn new(spec: &AgentSpec) -> Result<Self> {
        if spec.model.endpoint.is_none() {
            return Err(Error::InvalidSpec(
                "custom provider requires spec.model.endpoint".into(),
            ));
        }
        let api_key = std::env::var("CUSTOM_API_KEY")
            .map_err(|_| Error::Adapter("CUSTOM_API_KEY not set".into()))?;
        Self::new_with_key(spec, api_key)
    }

    pub fn new_with_key(spec: &AgentSpec, api_key: String) -> Result<Self> {
        if spec.model.endpoint.is_none() {
            return Err(Error::InvalidSpec(
                "custom provider requires spec.model.endpoint".into(),
            ));
        }
        let inner = OpenAIAdapter::new_with_key(spec, api_key)?;
        Ok(Self { inner })
    }

    pub fn with_client(mut self, client: reqwest::Client) -> Self {
        self.inner = self.inner.with_client(client);
        self
    }

    pub fn with_tools(mut self, tools: ToolRegistry) -> Self {
        self.inner = self.inner.with_tools(tools);
        self
    }
}

#[async_trait]
impl Adapter for CustomAdapter {
    async fn prepare(&self, config: &Value) -> Result<RuntimeContext> {
        self.inner.prepare(config).await
    }

    async fn invoke(
        &self,
        input: &Value,
        ctx: &RuntimeContext,
    ) -> Result<BoxStream<'static, Chunk>> {
        self.inner.invoke(input, ctx).await
    }

    async fn teardown(&self, ctx: &RuntimeContext) -> Result<()> {
        self.inner.teardown(ctx).await
    }

    async fn health(&self) -> HealthStatus {
        self.inner.health().await
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use hnsx_core::agent::{AdapterConfig, ModelRef, PromptTemplate, Provider};
    use serde_json::json;

    #[test]
    fn requires_endpoint() {
        let spec = AgentSpec {
            id: "a".into(),
            description: "x".into(),
            model: ModelRef {
                provider: Provider::Custom,
                model: "my-model".into(),
                endpoint: None,
            },
            adapter: AdapterConfig {
                timeout_seconds: None,
                extra: json!({}),
            },
            tools: vec![],
            prompt: PromptTemplate {
                template: "t".into(),
                variables: json!({}),
            },
            sandbox: None,
            memory_window: None,
        };
        let err = CustomAdapter::new_with_key(&spec, "k".into()).unwrap_err();
        assert!(matches!(err, Error::InvalidSpec(_)), "got: {err:?}");
    }
}
