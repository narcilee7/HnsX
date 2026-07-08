//! Native Ollama API adapter.
//!
//! Uses the Ollama `/api/chat` endpoint with NDJSON streaming, providing
//! real token/cost reporting. Cost is reported as 0 because Ollama is
//! self-hosted. Tool calling is supported using OpenAI-compatible function
//! definitions.

use std::time::Duration;

use async_stream::stream;
use async_trait::async_trait;
use futures::stream::{BoxStream, StreamExt};
use serde_json::{Value, json};

use crate::http_common::{estimate_tokens, value_to_string};
use crate::tool_chat::{MAX_TOOL_ROUNDS, execute_tool, tool_definitions};
use hnsx_core::adapter::{Adapter, RuntimeContext};
use hnsx_core::agent::{AgentSpec, HealthStatus};
use hnsx_core::chunk::{Artifact, Chunk};
use hnsx_core::error::{Error, Result};
use hnsx_core::tool::ToolRegistry;

const DEFAULT_TIMEOUT_SECONDS: u64 = 300;
const OLLAMA_BASE_URL: &str = "http://localhost:11434";

pub struct OllamaAdapter {
    client: reqwest::Client,
    model: String,
    system: String,
    base_url: String,
    timeout: Duration,
    tools: Option<ToolRegistry>,
    tool_defs: Option<Value>,
}

impl OllamaAdapter {
    pub fn new(spec: &AgentSpec) -> Result<Self> {
        let model = spec.model.model.clone();
        let system = spec.prompt.template.clone();
        let base_url = spec
            .model
            .endpoint
            .clone()
            .unwrap_or_else(|| OLLAMA_BASE_URL.into());
        let timeout = Duration::from_secs(
            spec.adapter
                .timeout_seconds
                .unwrap_or(DEFAULT_TIMEOUT_SECONDS),
        );
        let client = reqwest::Client::builder()
            .timeout(timeout)
            .build()
            .map_err(|e| Error::Adapter(format!("reqwest client build: {e}")))?;
        Ok(Self {
            client,
            model,
            system,
            base_url,
            timeout,
            tools: None,
            tool_defs: None,
        })
    }

    pub fn with_client(mut self, client: reqwest::Client) -> Self {
        self.client = client;
        self
    }

    pub fn with_tools(mut self, tools: ToolRegistry) -> Self {
        if !tools.is_empty() {
            self.tool_defs = tool_definitions(&tools);
            self.tools = Some(tools);
        }
        self
    }

    fn build_messages(&self, input: &Value) -> Vec<Value> {
        let mut messages = Vec::new();
        messages.push(json_message("system", Value::String(self.system.clone())));

        if let Some(mem) = input.get("_memory").and_then(Value::as_array) {
            for m in mem {
                if let (Some(role), Some(content)) = (
                    m.get("role").and_then(Value::as_str),
                    m.get("content").and_then(Value::as_str),
                ) {
                    messages.push(json_message(role, Value::String(content.to_string())));
                }
            }
        }

        let user_content = build_user_content(input);
        if !user_content.is_empty() {
            messages.push(json_message("user", Value::String(user_content)));
        }

        messages
    }

    fn has_tools(&self) -> bool {
        self.tools.is_some() && self.tool_defs.is_some()
    }

    async fn invoke_with_tools(&self, input: &Value) -> Result<BoxStream<'static, Chunk>> {
        let mut messages = self.build_messages(input);
        let tool_defs = self
            .tool_defs
            .clone()
            .expect("invoke_with_tools requires tools");
        let registry = self
            .tools
            .clone()
            .expect("invoke_with_tools requires tools");
        let client = self.client.clone();
        let model = self.model.clone();
        let system = self.system.clone();
        let url = format!("{}/api/chat", self.base_url.trim_end_matches('/'));

        Ok(Box::pin(stream! {
            let mut prompt_tokens: u64 = 0;
            let mut completion_tokens: u64 = 0;
            let mut final_text = String::new();

            for _round in 0..=MAX_TOOL_ROUNDS {
                let body = serde_json::json!({
                    "model": model,
                    "messages": messages,
                    "stream": true,
                    "tools": tool_defs,
                });

                let request = client
                    .post(&url)
                    .header("Content-Type", "application/json")
                    .json(&body);

                let response = match request.send().await {
                    Ok(r) => r,
                    Err(e) => {
                        yield Chunk::error(format!("Ollama request failed: {e}"));
                        return;
                    }
                };

                let status = response.status();
                if !status.is_success() {
                    let text = response.text().await.unwrap_or_default();
                    yield Chunk::error(format!("Ollama error ({status}): {text}"));
                    return;
                }

                let mut byte_stream = response.bytes_stream();
                let mut buffer = String::new();
                let mut assistant_text = String::new();
                let mut tool_calls: Vec<OllamaToolCall> = Vec::new();

                while let Some(chunk_result) = byte_stream.next().await {
                    let bytes = match chunk_result {
                        Ok(b) => b,
                        Err(e) => {
                            yield Chunk::error(format!("Ollama stream error: {e}"));
                            return;
                        }
                    };
                    buffer.push_str(&String::from_utf8_lossy(&bytes));

                    while let Some(pos) = buffer.find('\n') {
                        let line = buffer[..pos].trim().to_string();
                        buffer = buffer[pos + 1..].to_string();
                        if line.is_empty() {
                            continue;
                        }
                        let json: Value = match serde_json::from_str(&line) {
                            Ok(j) => j,
                            Err(e) => {
                                yield Chunk::error(format!("invalid NDJSON: {e}"));
                                return;
                            }
                        };

                        if let Some(done) = json.get("done").and_then(Value::as_bool) {
                            if done {
                                prompt_tokens = json
                                    .get("prompt_eval_count")
                                    .and_then(Value::as_u64)
                                    .unwrap_or(prompt_tokens);
                                completion_tokens = json
                                    .get("eval_count")
                                    .and_then(Value::as_u64)
                                    .unwrap_or(completion_tokens);
                            }
                        }

                        if let Some(message) = json.get("message") {
                            if let Some(text) = message.get("content").and_then(Value::as_str) {
                                assistant_text.push_str(text);
                            }
                            if let Some(calls) = message.get("tool_calls").and_then(Value::as_array) {
                                for call in calls {
                                    if let Some(func) = call.get("function") {
                                        let name = func
                                            .get("name")
                                            .and_then(Value::as_str)
                                            .unwrap_or("")
                                            .to_string();
                                        let arguments = func
                                            .get("arguments")
                                            .cloned()
                                            .unwrap_or_else(|| Value::Object(serde_json::Map::new()));
                                        tool_calls.push(OllamaToolCall { name, arguments });
                                    }
                                }
                            }
                        }
                    }
                }

                if !tool_calls.is_empty() {
                    messages.push(json!({
                        "role": "assistant",
                        "content": assistant_text,
                    }));
                    for call in tool_calls {
                        let result = execute_tool(&registry, &call.name, call.arguments.clone()).await;
                        let content = serde_json::to_string(&result)
                            .unwrap_or_else(|e| format!("{{\"error\":\"{e}\"}}"));
                        messages.push(json!({
                            "role": "tool",
                            "name": call.name,
                            "content": content,
                        }));
                    }
                    continue;
                }

                final_text = assistant_text;
                break;
            }

            if !final_text.is_empty() {
                yield Chunk::text(final_text);
            }

            if prompt_tokens == 0 {
                prompt_tokens = estimate_tokens(&system);
            }

            if prompt_tokens > 0 || completion_tokens > 0 {
                yield Chunk::artifact(Artifact::TokenUsage {
                    prompt: prompt_tokens,
                    completion: completion_tokens,
                    cost_usd: 0.0,
                });
            }
        }))
    }
}

#[async_trait]
impl Adapter for OllamaAdapter {
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
        if self.has_tools() {
            return self.invoke_with_tools(input).await;
        }

        let messages = self.build_messages(input);
        let body = serde_json::json!({
            "model": self.model,
            "messages": messages,
            "stream": true,
        });

        let url = format!("{}/api/chat", self.base_url.trim_end_matches('/'));
        let request = self
            .client
            .post(&url)
            .header("Content-Type", "application/json")
            .json(&body);

        let response = request
            .send()
            .await
            .map_err(|e| Error::Adapter(format!("Ollama request failed: {e}")))?;

        let status = response.status();
        if !status.is_success() {
            let text = response.text().await.unwrap_or_default();
            return Err(Error::Adapter(format!("Ollama error ({status}): {text}")));
        }

        let _model = self.model.clone();
        let system = self.system.clone();

        Ok(Box::pin(stream! {
            let mut byte_stream = response.bytes_stream();
            let mut prompt_tokens: u64 = 0;
            let mut completion_tokens: u64 = 0;
            let mut buffer = String::new();

            while let Some(chunk_result) = byte_stream.next().await {
                let bytes = match chunk_result {
                    Ok(b) => b,
                    Err(e) => {
                        yield Chunk::error(format!("Ollama stream error: {e}"));
                        return;
                    }
                };
                buffer.push_str(&String::from_utf8_lossy(&bytes));

                while let Some(pos) = buffer.find('\n') {
                    let line = buffer[..pos].trim().to_string();
                    buffer = buffer[pos + 1..].to_string();
                    if line.is_empty() {
                        continue;
                    }
                    let json: Value = match serde_json::from_str(&line) {
                        Ok(j) => j,
                        Err(e) => {
                            yield Chunk::error(format!("invalid NDJSON: {e}"));
                            return;
                        }
                    };

                    if let Some(done) = json.get("done").and_then(Value::as_bool) {
                        if done {
                            prompt_tokens = json
                                .get("prompt_eval_count")
                                .and_then(Value::as_u64)
                                .unwrap_or(prompt_tokens);
                            completion_tokens = json
                                .get("eval_count")
                                .and_then(Value::as_u64)
                                .unwrap_or(completion_tokens);
                        }
                    }

                    if let Some(message) = json.get("message") {
                        if let Some(text) = message.get("content").and_then(Value::as_str) {
                            if !text.is_empty() {
                                yield Chunk::text(text);
                            }
                        }
                    }
                }
            }

            if prompt_tokens == 0 {
                prompt_tokens = estimate_tokens(&system);
            }

            if prompt_tokens > 0 || completion_tokens > 0 {
                yield Chunk::artifact(Artifact::TokenUsage {
                    prompt: prompt_tokens,
                    completion: completion_tokens,
                    cost_usd: 0.0,
                });
            }
        }))
    }

    async fn teardown(&self, _ctx: &RuntimeContext) -> Result<()> {
        Ok(())
    }

    async fn health(&self) -> HealthStatus {
        let url = format!("{}/api/tags", self.base_url.trim_end_matches('/'));
        match self
            .client
            .get(&url)
            .timeout(Duration::from_secs(10))
            .send()
            .await
        {
            Ok(resp) if resp.status().is_success() => HealthStatus {
                healthy: true,
                message: Some(format!("Ollama adapter for {}", self.model)),
            },
            Ok(resp) => HealthStatus {
                healthy: false,
                message: Some(format!("Ollama health check failed: {}", resp.status())),
            },
            Err(e) => HealthStatus {
                healthy: false,
                message: Some(format!("Ollama health check error: {e}")),
            },
        }
    }
}

fn json_message(role: &str, content: Value) -> Value {
    serde_json::json!({
        "role": role,
        "content": content
    })
}

fn build_user_content(input: &Value) -> String {
    if let Some(s) = input.as_str() {
        return s.to_string();
    }
    if let Some(map) = input.as_object() {
        let filtered: serde_json::Map<String, Value> = map
            .iter()
            .filter(|(k, _)| *k != "_memory")
            .map(|(k, v)| (k.clone(), v.clone()))
            .collect();
        if filtered.is_empty() {
            return String::new();
        }
        return serde_json::to_string(&Value::Object(filtered)).unwrap_or_default();
    }
    value_to_string(input)
}

#[derive(Debug)]
struct OllamaToolCall {
    name: String,
    arguments: Value,
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn spec(model: &str) -> AgentSpec {
        use hnsx_core::agent::{AdapterConfig, ModelRef, PromptTemplate, Provider};
        AgentSpec {
            id: "a".into(),
            description: "x".into(),
            model: ModelRef {
                provider: Provider::Ollama,
                model: model.into(),
                endpoint: None,
            },
            adapter: AdapterConfig {
                timeout_seconds: Some(30),
                extra: json!({}),
            },
            tools: vec![],
            prompt: PromptTemplate {
                template: "You are a test assistant.".into(),
                variables: json!({}),
            },
            sandbox: None,
            memory_window: None,
        }
    }

    #[test]
    fn build_messages_includes_system() {
        let adapter = OllamaAdapter::new(&spec("llama3.1")).unwrap();
        let messages = adapter.build_messages(&json!("hi"));
        assert_eq!(messages.len(), 2);
        assert_eq!(messages[0]["role"], "system");
        assert_eq!(messages[1]["role"], "user");
    }
}
