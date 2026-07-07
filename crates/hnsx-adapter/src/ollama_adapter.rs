//! Native Ollama API adapter.
//!
//! Uses the Ollama `/api/chat` endpoint with NDJSON streaming, providing
//! real token/cost reporting. Cost is reported as 0 because Ollama is
//! self-hosted.

use std::time::Duration;

use async_stream::stream;
use async_trait::async_trait;
use futures::stream::{BoxStream, StreamExt};
use serde_json::Value;

use crate::http_common::{value_to_string, estimate_tokens};
use hnsx_core::adapter::{Adapter, RuntimeContext};
use hnsx_core::agent::{AgentSpec, HealthStatus};
use hnsx_core::chunk::{Artifact, Chunk};
use hnsx_core::error::{Error, Result};

const DEFAULT_TIMEOUT_SECONDS: u64 = 300;
const OLLAMA_BASE_URL: &str = "http://localhost:11434";

pub struct OllamaAdapter {
    client: reqwest::Client,
    model: String,
    system: String,
    base_url: String,
    timeout: Duration,
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
        let timeout = Duration::from_secs(spec.adapter.timeout_seconds.unwrap_or(DEFAULT_TIMEOUT_SECONDS));
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
        })
    }

    pub fn with_client(mut self, client: reqwest::Client) -> Self {
        self.client = client;
        self
    }

    fn build_messages(&self, input: &Value) -> Vec<Value> {
        let mut messages = Vec::new();
        messages.push(json_message("system", &self.system));

        if let Some(mem) = input.get("_memory").and_then(Value::as_array) {
            for m in mem {
                if let (Some(role), Some(content)) = (
                    m.get("role").and_then(Value::as_str),
                    m.get("content").and_then(Value::as_str),
                ) {
                    messages.push(json_message(role, content));
                }
            }
        }

        let user_content = build_user_content(input);
        if !user_content.is_empty() {
            messages.push(json_message("user", &user_content));
        }

        messages
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

    async fn invoke(&self, input: &Value, _ctx: &RuntimeContext) -> Result<BoxStream<'static, Chunk>> {
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

fn json_message(role: &str, content: &str) -> Value {
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
