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
//!
//! Phase 3 adds tool use: tools declared in `AgentSpec.tools` are converted
//! to `genai::chat::Tool` definitions and the agent runs a request → tool
//! call → response loop, yielding assistant text as it streams.

use std::sync::Arc;

use async_stream::stream;
use async_trait::async_trait;
use futures::stream::{BoxStream, StreamExt};
use genai::Client;
use genai::chat::{
    ChatMessage, ChatOptions, ChatRequest, ChatStreamEvent, StreamEnd, ToolCall, ToolResponse,
};
use serde_json::Value;

use crate::tools::{build_tool_registry, to_genai_tool};
use hnsx_core::agent::{Agent, AgentSpec, HealthStatus, InvokeContext, Provider, ToolKind};
use hnsx_core::agent_factory::AgentFactory;
use hnsx_core::chunk::Chunk;
use hnsx_core::error::{Error, Result};
use hnsx_core::tool::ToolRegistry;

const MAX_TOOL_ROUNDS: usize = 5;

/// A single genai-backed agent: one model, one system prompt, optional tools.
pub struct GenaiAgent {
    client: Client,
    /// The fully-qualified genai model name, e.g. `openai::gpt-4o-mini`.
    model: String,
    /// The system prompt (taken from `AgentSpec.prompt.template`).
    system: String,
    /// Tools this agent may invoke.
    tools: ToolRegistry,
    /// Maximum number of tool-call rounds before the loop is cut off.
    max_tool_rounds: usize,
}

impl GenaiAgent {
    pub fn new(
        client: Client,
        model: String,
        system: String,
        tools: ToolRegistry,
    ) -> Self {
        Self {
            client,
            model,
            system,
            tools,
            max_tool_rounds: MAX_TOOL_ROUNDS,
        }
    }

    /// Build the genai `Tool` definitions from the registry, if any.
    fn genai_tools(&self) -> Option<Vec<genai::chat::Tool>> {
        if self.tools.is_empty() {
            return None;
        }
        let tools: Vec<_> = self
            .tools
            .iter()
            .filter_map(|(_, _, tool)| to_genai_tool(tool.as_ref()))
            .collect();
        if tools.is_empty() {
            None
        } else {
            Some(tools)
        }
    }
}

#[async_trait]
impl Agent for GenaiAgent {
    async fn invoke(&self, input: Value, _ctx: InvokeContext) -> Result<BoxStream<'static, Chunk>> {
        let tools = self.genai_tools();
        let mut messages = vec![ChatMessage::user(format!("{input}"))];

        let client = self.client.clone();
        let model = self.model.clone();
        let system = self.system.clone();
        let registry = self.tools.clone();
        let max_rounds = self.max_tool_rounds;
        let options = ChatOptions::default()
            .with_capture_content(true)
            .with_capture_tool_calls(true);

        Ok(Box::pin(stream! {
            for _round in 0..=max_rounds {
                let mut chat_req = ChatRequest::new(messages.clone())
                    .with_system(system.clone());
                if let Some(ref t) = tools {
                    chat_req = chat_req.with_tools(t.clone());
                }

                let stream_res = match client.exec_chat_stream(&model, chat_req, Some(&options)).await {
                    Ok(res) => res,
                    Err(e) => {
                        yield Chunk::error(format!("genai: {e}"));
                        return;
                    }
                };

                let mut stream = stream_res.stream;
                let mut end_event: Option<StreamEnd> = None;
                while let Some(event) = stream.next().await {
                    match event {
                        Ok(ChatStreamEvent::Chunk(chunk)) => {
                            if !chunk.content.is_empty() {
                                yield Chunk::text(chunk.content);
                            }
                        }
                        Ok(ChatStreamEvent::ToolCallChunk(_)) => {
                            // Tool calls are collected and reported in the End event.
                        }
                        Ok(ChatStreamEvent::End(end)) => {
                            end_event = Some(end);
                        }
                        Ok(ChatStreamEvent::Start)
                        | Ok(ChatStreamEvent::ReasoningChunk(_))
                        | Ok(ChatStreamEvent::ThoughtSignatureChunk(_)) => {
                            // Phase 3 ignores non-text / non-tool events.
                        }
                        Err(e) => {
                            yield Chunk::error(format!("genai stream error: {e}"));
                            return;
                        }
                    }
                }

                let end = match end_event {
                    Some(end) => end,
                    None => break,
                };

                // If the model did not request any tool calls, we are done.
                let assistant_msg = match end.into_assistant_message_for_tool_use() {
                    Some(msg) => msg,
                    None => break,
                };

                let tool_calls: Vec<ToolCall> = assistant_msg
                    .content
                    .tool_calls()
                    .into_iter()
                    .cloned()
                    .collect();
                if tool_calls.is_empty() {
                    break;
                }

                messages.push(assistant_msg);

                // Execute each tool call and build tool-response messages.
                let mut responses = Vec::with_capacity(tool_calls.len());
                for call in tool_calls {
                    let content = match execute_tool_call(&registry, &call).await {
                        Ok(value) => serde_json::to_string(&value)
                            .unwrap_or_else(|e| format!("{{\"error\":\"{e}\"}}")),
                        Err(e) => format!("{{\"error\":\"{e}\"}}"),
                    };
                    responses.push(ToolResponse::from_tool_call(&call, content));
                }
                messages.push(ChatMessage::from(responses));
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

/// Execute a single tool call against the registry.
///
/// The `genai` tool name is a plain string, so we try each `ToolKind` bucket
/// in turn. The first match wins.
async fn execute_tool_call(registry: &ToolRegistry, call: &ToolCall) -> Result<Value> {
    for kind in [ToolKind::Http, ToolKind::Python, ToolKind::Shell, ToolKind::Sql] {
        if let Some(tool) = registry.get(kind, &call.fn_name) {
            return tool.invoke(call.fn_arguments.clone()).await;
        }
    }
    Err(Error::Adapter(format!(
        "tool `{}` not found in registry",
        call.fn_name
    )))
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
        let tools = build_tool_registry(&spec.tools)?;
        Ok(Arc::new(GenaiAgent::new(
            self.client.clone(),
            model,
            system,
            tools,
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
            memory_window: None,
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
