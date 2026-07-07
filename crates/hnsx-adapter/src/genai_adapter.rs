//! `GenaiAdapter`: wrap the `genai` multi-provider client behind the
//! `hnsx_core::Adapter` trait so it can be composed into an `HnsXAgent`.
//!
//! This replaces the older `GenaiAgent` (which implemented `Agent` directly).
//! The adapter is stateless apart from the `genai::Client`; prepare/teardown
//! are no-ops.

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
use hnsx_core::adapter::{Adapter, RuntimeContext};
use hnsx_core::agent::{AgentSpec, HealthStatus, ToolKind};
use hnsx_core::chunk::Chunk;
use hnsx_core::error::{Error, Result};
use hnsx_core::tool::ToolRegistry;

const MAX_TOOL_ROUNDS: usize = 5;

/// Adapter backed by the `genai` multi-provider client.
pub struct GenaiAdapter {
    client: Client,
    model: String,
    system: String,
    tools: ToolRegistry,
}

impl GenaiAdapter {
    pub fn new(client: Client, model: String, system: String, tools: ToolRegistry) -> Self {
        Self {
            client,
            model,
            system,
            tools,
        }
    }

    fn genai_tools(&self) -> Option<Vec<genai::chat::Tool>> {
        if self.tools.is_empty() {
            return None;
        }
        let tools: Vec<_> = self
            .tools
            .iter()
            .filter_map(|(_, _, tool)| to_genai_tool(tool.as_ref()))
            .collect();
        if tools.is_empty() { None } else { Some(tools) }
    }
}

#[async_trait]
impl Adapter for GenaiAdapter {
    async fn prepare(&self, _config: &Value) -> Result<RuntimeContext> {
        Ok(RuntimeContext {
            session_id: uuid::Uuid::new_v4().to_string(),
            config: Value::Null,
        })
    }

    async fn invoke(
        &self,
        input: &Value,
        _ctx: &RuntimeContext,
    ) -> Result<BoxStream<'static, Chunk>> {
        let tools = self.genai_tools();
        let mut messages = vec![ChatMessage::user(format!("{input}"))];

        let client = self.client.clone();
        let model = self.model.clone();
        let system = self.system.clone();
        let registry = self.tools.clone();
        let options = ChatOptions::default()
            .with_capture_content(true)
            .with_capture_tool_calls(true);

        Ok(Box::pin(stream! {
            for _round in 0..=MAX_TOOL_ROUNDS {
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
                        Ok(ChatStreamEvent::ToolCallChunk(_)) => {}
                        Ok(ChatStreamEvent::End(end)) => {
                            end_event = Some(end);
                        }
                        Ok(ChatStreamEvent::Start)
                        | Ok(ChatStreamEvent::ReasoningChunk(_))
                        | Ok(ChatStreamEvent::ThoughtSignatureChunk(_)) => {}
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

    async fn teardown(&self, _ctx: &RuntimeContext) -> Result<()> {
        Ok(())
    }

    async fn health(&self) -> HealthStatus {
        HealthStatus {
            healthy: true,
            message: Some(format!("genai adapter for {}", self.model)),
        }
    }
}

async fn execute_tool_call(registry: &ToolRegistry, call: &ToolCall) -> Result<Value> {
    for kind in [
        ToolKind::Http,
        ToolKind::Python,
        ToolKind::Shell,
        ToolKind::Sql,
    ] {
        if let Some(tool) = registry.get(kind, &call.fn_name) {
            return tool.invoke(call.fn_arguments.clone()).await;
        }
    }
    Err(Error::Adapter(format!(
        "tool `{}` not found in registry",
        call.fn_name
    )))
}

/// Factory that resolves an `AgentSpec` to a `GenaiAdapter`.
#[derive(Clone, Default)]
pub struct GenaiAdapterFactory {
    client: Client,
}

impl GenaiAdapterFactory {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn with_client(client: Client) -> Self {
        Self { client }
    }

    pub fn create_adapter(&self, spec: &AgentSpec) -> Result<Arc<dyn Adapter>> {
        let model = super::genai::genai_model_name(spec)?;
        let system = spec.prompt.template.clone();
        let tools = build_tool_registry(&spec.tools)?;
        Ok(Arc::new(GenaiAdapter::new(
            self.client.clone(),
            model,
            system,
            tools,
        )))
    }
}

impl hnsx_core::agent_factory::AgentFactory for GenaiAdapterFactory {
    fn create(&self, spec: &AgentSpec) -> Result<Arc<dyn hnsx_core::Agent>> {
        let model = super::genai::genai_model_name(spec)?;
        let system = spec.prompt.template.clone();
        let tools = build_tool_registry(&spec.tools)?;
        Ok(Arc::new(super::genai::GenaiAgent::new(
            self.client.clone(),
            model,
            system,
            tools,
        )))
    }
}

/// Backwards-compatible alias for the existing factory.
pub type GenaiAgentFactory = GenaiAdapterFactory;
