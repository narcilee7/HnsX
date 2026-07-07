//! `HnsXAgent`: the canonical Agent implementation that composes an
//! [`Adapter`], a [`Sandbox`], a [`MemoryBackend`], and an optional set of
//! tools into a single [`Agent`] trait object.

use std::sync::Arc;

use async_stream::stream;
use async_trait::async_trait;
use futures::stream::{BoxStream, StreamExt};
use serde_json::Value;

use crate::adapter::Adapter;
use crate::agent::{Agent, AgentSchema, AgentSpec, HealthStatus, InvokeContext};
use crate::chunk::Chunk;
use crate::error::{Error, Result};
use crate::memory::{MemoryBackend, Message};
use crate::sandbox::Sandbox;
use crate::tool::ToolRegistry;

/// A concrete Agent that wraps an adapter and attaches sandbox, memory, and
/// tools according to the agent spec.
pub struct HnsXAgent {
    spec: AgentSpec,
    adapter: Arc<dyn Adapter>,
    sandbox: Arc<dyn Sandbox>,
    memory: Option<Arc<dyn MemoryBackend>>,
    tool_registry: ToolRegistry,
    memory_window: usize,
}

impl HnsXAgent {
    pub fn new(
        spec: AgentSpec,
        adapter: Arc<dyn Adapter>,
        sandbox: Arc<dyn Sandbox>,
        memory: Option<Arc<dyn MemoryBackend>>,
        tool_registry: ToolRegistry,
    ) -> Self {
        let memory_window = spec.memory_window.unwrap_or(10);
        Self {
            spec,
            adapter,
            sandbox,
            memory,
            tool_registry,
            memory_window,
        }
    }
}

#[async_trait]
impl Agent for HnsXAgent {
    async fn invoke(&self, input: Value, ctx: InvokeContext) -> Result<BoxStream<'static, Chunk>> {
        let adapter = self.adapter.clone();
        let _sandbox = self.sandbox.clone();
        let memory = self.memory.clone();
        let tool_registry = self.tool_registry.clone();
        let agent_id = self.spec.id.clone();
        let memory_window = self.memory_window;

        // 1. Prepare adapter runtime context.
        let runtime_ctx = adapter
            .prepare(&serde_json::json!({
                "agent_id": agent_id,
                "has_tools": !tool_registry.is_empty(),
            }))
            .await?;

        // 2. Load memory context for this agent/session.
        let memory_messages: Vec<Message> = if let Some(mem) = &memory {
            let session = mem.load_session(&ctx.domain_id, &ctx.session_id).await?;
            mem.build_context(&session, &agent_id, memory_window)
                .await?
        } else {
            Vec::new()
        };

        // 3. Merge memory context into input.
        let enriched_input = if memory_messages.is_empty() {
            input
        } else {
            let mut map = match input {
                Value::Object(m) => m,
                other => {
                    let mut m = serde_json::Map::new();
                    m.insert("input".to_string(), other);
                    m
                }
            };
            let mem_val = serde_json::to_value(&memory_messages).unwrap_or_default();
            map.insert("_memory".to_string(), mem_val);
            Value::Object(map)
        };

        // 4. Invoke adapter.
        let mut stream = adapter.invoke(&enriched_input, &runtime_ctx).await?;

        // 5. Aggregate output and save turn.
        let output = Arc::new(std::sync::Mutex::new(String::new()));
        let output_clone = output.clone();
        let memory_clone = memory.clone();
        let ctx_clone = ctx.clone();
        let runtime_ctx_clone = runtime_ctx.clone();
        let adapter_clone = adapter.clone();

        let aggregate_stream = Box::pin(stream! {
            while let Some(chunk) = stream.next().await {
                if let Chunk::Text(text) = &chunk {
                    output_clone.lock().unwrap().push_str(text);
                }
                yield chunk;
            }

            // Save turn after the stream finishes.
            if let Some(mem) = memory_clone {
                let content = output_clone.lock().unwrap().clone();
                let session = mem.load_session(&ctx_clone.domain_id, &ctx_clone.session_id).await;
                if let Ok(session) = session {
                    if let Err(e) = mem.save_turn(&session, &ctx_clone.agent_id, "assistant", &content).await {
                        yield Chunk::error(format!("failed to save memory turn: {e}"));
                    }
                }
            }

            // Teardown adapter.
            if let Err(e) = adapter_clone.teardown(&runtime_ctx_clone).await {
                yield Chunk::error(format!("adapter teardown failed: {e}"));
            }
        });

        Ok(aggregate_stream)
    }

    async fn health(&self) -> HealthStatus {
        self.adapter.health().await
    }

    async fn schema(&self) -> AgentSchema {
        AgentSchema {
            name: self.spec.id.clone(),
            input_schema: serde_json::json!({"type": "object"}),
            output_schema: serde_json::json!({"type": "string"}),
        }
    }
}

/// Builder for `HnsXAgent`.
#[derive(Default)]
pub struct HnsXAgentBuilder {
    spec: Option<AgentSpec>,
    adapter: Option<Arc<dyn Adapter>>,
    sandbox: Option<Arc<dyn Sandbox>>,
    memory: Option<Arc<dyn MemoryBackend>>,
    tool_registry: Option<ToolRegistry>,
}

impl HnsXAgentBuilder {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn spec(mut self, spec: AgentSpec) -> Self {
        self.spec = Some(spec);
        self
    }

    pub fn adapter(mut self, adapter: Arc<dyn Adapter>) -> Self {
        self.adapter = Some(adapter);
        self
    }

    pub fn sandbox(mut self, sandbox: Arc<dyn Sandbox>) -> Self {
        self.sandbox = Some(sandbox);
        self
    }

    pub fn memory(mut self, memory: Arc<dyn MemoryBackend>) -> Self {
        self.memory = Some(memory);
        self
    }

    pub fn tools(mut self, tools: ToolRegistry) -> Self {
        self.tool_registry = Some(tools);
        self
    }

    pub fn build(self) -> Result<HnsXAgent> {
        let spec = self
            .spec
            .ok_or_else(|| Error::InvalidSpec("HnsXAgentBuilder: spec is required".into()))?;
        let adapter = self
            .adapter
            .ok_or_else(|| Error::InvalidSpec("HnsXAgentBuilder: adapter is required".into()))?;
        let sandbox = self
            .sandbox
            .ok_or_else(|| Error::InvalidSpec("HnsXAgentBuilder: sandbox is required".into()))?;
        let tool_registry = self.tool_registry.unwrap_or_default();
        Ok(HnsXAgent::new(
            spec,
            adapter,
            sandbox,
            self.memory,
            tool_registry,
        ))
    }
}
