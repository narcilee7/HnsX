//! Native OpenAI API adapter.
//!
//! Implements the `Adapter` trait directly over `reqwest` + SSE, providing
//! real token/cost reporting and provider-specific error classification.
//! Tool calling is intentionally not implemented here; domains that declare
//! tools fall back to `genai_adapter` until Phase 2.

use std::time::Duration;

use async_stream::stream;
use async_trait::async_trait;
use futures::stream::{BoxStream, StreamExt};
use serde_json::Value;

use crate::http_common::{
    bearer, classify_http_error, openai_cost, parse_sse_stream, value_to_string,
};
use hnsx_core::adapter::{Adapter, RuntimeContext};
use hnsx_core::agent::{AgentSpec, HealthStatus};
use hnsx_core::chunk::{Artifact, Chunk};
use hnsx_core::error::{Error, Result};

const DEFAULT_TIMEOUT_SECONDS: u64 = 300;
const OPENAI_BASE_URL: &str = "https://api.openai.com";

pub struct OpenAIAdapter {
    client: reqwest::Client,
    api_key: String,
    model: String,
    system: String,
    base_url: String,
    timeout: Duration,
}

impl OpenAIAdapter {
    pub fn new(spec: &AgentSpec) -> Result<Self> {
        let api_key = std::env::var("OPENAI_API_KEY")
            .map_err(|_| Error::Adapter("OPENAI_API_KEY not set".into()))?;
        Self::new_with_key(spec, api_key)
    }

    pub fn new_with_key(spec: &AgentSpec, api_key: String) -> Result<Self> {
        let model = spec.model.model.clone();
        let system = spec.prompt.template.clone();
        let base_url = spec
            .model
            .endpoint
            .clone()
            .unwrap_or_else(|| OPENAI_BASE_URL.into());
        let timeout = Duration::from_secs(spec.adapter.timeout_seconds.unwrap_or(DEFAULT_TIMEOUT_SECONDS));
        let client = reqwest::Client::builder()
            .timeout(timeout)
            .build()
            .map_err(|e| Error::Adapter(format!("reqwest client build: {e}")))?;
        Ok(Self {
            client,
            api_key,
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
impl Adapter for OpenAIAdapter {
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
            "stream_options": { "include_usage": true },
        });

        let url = format!("{}/v1/chat/completions", self.base_url.trim_end_matches('/'));
        let request = self
            .client
            .post(&url)
            .header("Authorization", bearer(&self.api_key))
            .header("Content-Type", "application/json")
            .json(&body);

        let response = request
            .send()
            .await
            .map_err(|e| Error::Adapter(format!("OpenAI request failed: {e}")))?;

        let status = response.status();
        if !status.is_success() {
            let text = response.text().await.unwrap_or_default();
            return Err(classify_http_error(status, &text));
        }

        let model = self.model.clone();
        let sse_stream = parse_sse_stream(response.bytes_stream());

        Ok(Box::pin(stream! {
            let mut sse_stream = sse_stream;
            let mut prompt_tokens: u64 = 0;
            let mut completion_tokens: u64 = 0;
            let mut emitted_text = false;

            while let Some(event_result) = sse_stream.next().await {
                let event = match event_result {
                    Ok(e) => e,
                    Err(e) => {
                        yield Chunk::error(format!("SSE parse error: {e}"));
                        return;
                    }
                };

                let json = match event.json() {
                    Ok(Value::Null) => continue,
                    Ok(j) => j,
                    Err(e) => {
                        yield Chunk::error(format!("invalid SSE JSON: {e}"));
                        return;
                    }
                };

                if let Some(usage) = json.get("usage") {
                    prompt_tokens = usage.get("prompt_tokens").and_then(Value::as_u64).unwrap_or(prompt_tokens);
                    completion_tokens = usage.get("completion_tokens").and_then(Value::as_u64).unwrap_or(completion_tokens);
                }

                if let Some(choices) = json.get("choices").and_then(Value::as_array) {
                    for choice in choices {
                        if let Some(delta) = choice.get("delta") {
                            if let Some(text) = delta.get("content").and_then(Value::as_str) {
                                if !text.is_empty() {
                                    yield Chunk::text(text);
                                    emitted_text = true;
                                }
                            }
                        }
                    }
                }
            }

            if !emitted_text && prompt_tokens == 0 && completion_tokens == 0 {
                // No usage reported and no text; estimate from what we streamed.
                // We can't reconstitute the text here easily, so leave usage at 0.
            }

            let cost_usd = openai_cost(&model, prompt_tokens, completion_tokens);
            if prompt_tokens > 0 || completion_tokens > 0 {
                yield Chunk::artifact(Artifact::TokenUsage {
                    prompt: prompt_tokens,
                    completion: completion_tokens,
                    cost_usd,
                });
            }
        }))
    }

    async fn teardown(&self, _ctx: &RuntimeContext) -> Result<()> {
        Ok(())
    }

    async fn health(&self) -> HealthStatus {
        let url = format!("{}/v1/models", self.base_url.trim_end_matches('/'));
        match self
            .client
            .get(&url)
            .header("Authorization", bearer(&self.api_key))
            .timeout(Duration::from_secs(10))
            .send()
            .await
        {
            Ok(resp) if resp.status().is_success() => HealthStatus {
                healthy: true,
                message: Some(format!("OpenAI adapter for {}", self.model)),
            },
            Ok(resp) => HealthStatus {
                healthy: false,
                message: Some(format!("OpenAI health check failed: {}", resp.status())),
            },
            Err(e) => HealthStatus {
                healthy: false,
                message: Some(format!("OpenAI health check error: {e}")),
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
    // If input is a plain string, use it directly.
    if let Some(s) = input.as_str() {
        return s.to_string();
    }

    // If it's an object, drop the memory field and serialize the rest.
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
                provider: Provider::Openai,
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
    fn build_messages_with_memory() {
        let adapter = OpenAIAdapter::new_with_key(&spec("gpt-4o-mini"), "test-key".into()).unwrap();
        let input = json!({
            "task": "hello",
            "_memory": [
                {"role": "user", "content": "previous"}
            ]
        });
        let messages = adapter.build_messages(&input);
        assert_eq!(messages.len(), 3);
        assert_eq!(messages[0]["role"], "system");
        assert_eq!(messages[1]["role"], "user");
        assert_eq!(messages[1]["content"], "previous");
        assert_eq!(messages[2]["role"], "user");
    }

    #[test]
    fn build_messages_plain_string() {
        let adapter = OpenAIAdapter::new_with_key(&spec("gpt-4o-mini"), "test-key".into()).unwrap();
        let messages = adapter.build_messages(&json!("hi"));
        assert_eq!(messages.len(), 2);
        assert_eq!(messages[1]["content"], "hi");
    }
}
