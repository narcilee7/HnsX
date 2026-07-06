//! `GenaiAgent` + `GenaiAgentFactory`: wrap the `genai` multi-provider client
//! behind the `hnsx_core::Agent` and `hnsx_core::AgentFactory` traits.
//!
//! Phase 1.4 lands the OpenAI / Ollama / custom-endpoint path; 1.5 will
//! extend coverage to Anthropic without changing the surface (the Provider
//! enum already names it).
//!
//! The model name passed to `genai` is the canonical `provider::model`
//! form (e.g. `openai::gpt-4o-mini`, `ollama::llama3.1`). For
//! `Provider::Custom` we expect the spec to carry the full model name
//! (`genai_1::something`), with `GENAI_1_ENDPOINT` pointing at the
//! upstream base URL.

use std::sync::Arc;

use async_stream::stream;
use async_trait::async_trait;
use futures::stream::{BoxStream, StreamExt};
use genai::Client;
use genai::chat::{ChatMessage, ChatRequest, ChatStreamEvent};
use serde_json::Value;

use hnsx_core::agent::{Agent, AgentSpec, HealthStatus, InvokeContext, Provider};
use hnsx_core::agent_factory::AgentFactory;
use hnsx_core::chunk::Chunk;
use hnsx_core::error::{Error, Result};

/// A single genai-backed agent: one model, one system prompt, runs forever.
pub struct GenaiAgent {
    client: Client,
    /// The fully-qualified genai model name, e.g. `openai::gpt-4o-mini`.
    model: String,
    /// The system prompt (taken from `AgentSpec.prompt.template`).
    system: String,
}

impl GenaiAgent {
    pub fn new(client: Client, model: String, system: String) -> Self {
        Self {
            client,
            model,
            system,
        }
    }
}

#[async_trait]
impl Agent for GenaiAgent {
    async fn invoke(&self, input: Value, _ctx: InvokeContext) -> Result<BoxStream<'static, Chunk>> {
        let chat_req = ChatRequest::new(vec![ChatMessage::user(format!("{input}"))])
            .with_system(self.system.clone());

        let stream_res = self
            .client
            .exec_chat_stream(&self.model, chat_req, None)
            .await
            .map_err(|e| Error::Adapter(format!("genai: {e}")))?;

        let mut stream = stream_res.stream;
        Ok(Box::pin(stream! {
            while let Some(event) = stream.next().await {
                match event {
                    Ok(ChatStreamEvent::Chunk(chunk)) => {
                        if !chunk.content.is_empty() {
                            yield Chunk::text(chunk.content);
                        }
                    }
                    Ok(ChatStreamEvent::ReasoningChunk(_))
                    | Ok(ChatStreamEvent::ThoughtSignatureChunk(_))
                    | Ok(ChatStreamEvent::ToolCallChunk(_)) => {
                        // 1.4 ignores non-text events. Tools land in Phase 3.
                    }
                    Ok(ChatStreamEvent::Start) | Ok(ChatStreamEvent::End(_)) => {
                        // Bookends; no payload to surface.
                    }
                    Err(e) => {
                        yield Chunk::error(format!("genai stream error: {e}"));
                        return;
                    }
                }
            }
        }))
    }

    async fn health(&self) -> HealthStatus {
        // For 1.4 we report healthy if we got here. A real probe lands with
        // Phase 5 (control plane) where the agent also reports latencies.
        HealthStatus {
            healthy: true,
            message: Some(format!("genai agent for {}", self.model)),
        }
    }

    async fn schema(&self) -> hnsx_core::agent::AgentSchema {
        hnsx_core::agent::AgentSchema {
            name: self.model.clone(),
            input_schema: serde_json::json!({"type": "object"}),
            output_schema: serde_json::json!({"type": "string"}),
        }
    }
}

/// Resolves an `AgentSpec` to a `GenaiAgent`.
///
/// Build with [`GenaiAgentFactory::default`] (reads API keys from env and
/// discovers endpoints from `GENAI_*_ENDPOINT`) or [`GenaiAgentFactory::with_client`]
/// for tests that want a custom `reqwest::Client` (e.g. `wiremock`).
#[derive(Clone, Default)]
pub struct GenaiAgentFactory {
    client: Client,
}

impl GenaiAgentFactory {
    pub fn new() -> Self {
        Self::default()
    }

    /// Construct a factory that uses `client` instead of the default. Tests
    /// pass a `reqwest::Client` with a custom base URL through here.
    pub fn with_client(client: Client) -> Self {
        Self { client }
    }
}

impl AgentFactory for GenaiAgentFactory {
    fn create(&self, spec: &AgentSpec) -> Result<Arc<dyn Agent>> {
        let model = genai_model_name(spec)?;
        let system = spec.prompt.template.clone();
        Ok(Arc::new(GenaiAgent::new(
            self.client.clone(),
            model,
            system,
        )))
    }
}

/// Map `(Provider, ModelRef.model)` to a fully-qualified `genai` model name.
///
/// For `Provider::Custom` we pass through the user-provided string as-is; the
/// caller is responsible for setting up `GENAI_N_ENDPOINT` (or using
/// `Client::with_reqwest` for an explicit base URL).
pub fn genai_model_name(spec: &AgentSpec) -> Result<String> {
    let model = &spec.model.model;
    let provider = spec.model.provider;
    let explicit = spec
        .model
        .endpoint
        .as_deref()
        .map(str::trim)
        .filter(|s| !s.is_empty());

    // If the user provided a full genai-style name (e.g. `genai_1::llama3`)
    // directly in `model`, take it verbatim. This is the cleanest way to
    // express "custom OpenAI-compatible endpoint" without a separate config
    // block.
    if let Some(s) = explicit {
        // `endpoint` doubles as "fully qualified model name" for custom.
        let _ = s;
    }

    let name = match provider {
        Provider::Openai => format!("openai::{model}"),
        Provider::Anthropic => format!("anthropic::{model}"),
        Provider::Ollama => format!("ollama::{model}"),
        // Codex CLI speaks the OpenAI protocol, so we route it through
        // genai's openai adapter. The actual CLI shim lands separately
        // in Phase 4.2; for 1.4 the model string is just informational.
        Provider::Codex => format!("openai::{model}"),
        // Claude Code CLI is a sandboxed CLI shim, not an HTTP API.
        // It lands as a real Adapter in Phase 2. For 1.4 we error out
        // cleanly so the user knows to switch to `provider: anthropic`.
        Provider::ClaudeCode => {
            return Err(Error::Adapter(
                "claude-code provider requires the hnsx-sandbox-backed \
                 ClaudeCodeAdapter, which lands in Phase 2. Use \
                 `provider: anthropic` to talk to the Anthropic API directly."
                    .to_string(),
            ));
        }
        // `Custom` is a free-form OpenAI-compatible endpoint. The model
        // field is expected to already be a fully-qualified genai name
        // (e.g. `genai_1::llama3`). If it isn't, treat the whole string
        // as a model name under `genai_1`.
        Provider::Custom => {
            if model.contains("::") {
                model.clone()
            } else {
                format!("genai_1::{model}")
            }
        }
    };
    Ok(name)
}

#[cfg(test)]
mod tests {
    use super::*;
    use hnsx_core::agent::{AdapterConfig, ModelRef, PromptTemplate};
    use serde_json::json;

    fn spec(provider: Provider, model: &str) -> AgentSpec {
        AgentSpec {
            id: "a".into(),
            description: "x".into(),
            model: ModelRef {
                provider,
                model: model.into(),
                endpoint: None,
            },
            adapter: AdapterConfig {
                timeout_seconds: None,
                extra: json!({}),
            },
            tools: vec![],
            prompt: PromptTemplate {
                template: "you are a test".into(),
                variables: json!({}),
            },
            sandbox: None,
        }
    }

    #[test]
    fn maps_openai_provider() {
        assert_eq!(
            genai_model_name(&spec(Provider::Openai, "gpt-4o-mini")).unwrap(),
            "openai::gpt-4o-mini"
        );
    }

    #[test]
    fn maps_anthropic_provider() {
        assert_eq!(
            genai_model_name(&spec(Provider::Anthropic, "claude-haiku-4-5")).unwrap(),
            "anthropic::claude-haiku-4-5"
        );
    }

    #[test]
    fn maps_ollama_provider() {
        assert_eq!(
            genai_model_name(&spec(Provider::Ollama, "llama3.1")).unwrap(),
            "ollama::llama3.1"
        );
    }

    #[test]
    fn custom_passes_through_fqn() {
        assert_eq!(
            genai_model_name(&spec(Provider::Custom, "genai_1::llama3")).unwrap(),
            "genai_1::llama3"
        );
    }

    #[test]
    fn custom_prefixes_when_unqualified() {
        assert_eq!(
            genai_model_name(&spec(Provider::Custom, "my-llama")).unwrap(),
            "genai_1::my-llama"
        );
    }
}
