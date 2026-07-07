//! Native Anthropic API adapter.
//!
//! Implements the `Adapter` trait directly over `reqwest` + SSE for the
//! Anthropic Messages API, providing real token/cost reporting and
//! provider-specific error classification.

use std::time::Duration;

use async_stream::stream;
use async_trait::async_trait;
use futures::stream::{BoxStream, StreamExt};
use serde_json::Value;

use crate::http_common::{
    classify_http_error, estimate_tokens, parse_sse_stream, value_to_string,
};
use hnsx_core::adapter::{Adapter, RuntimeContext};
use hnsx_core::agent::{AgentSpec, HealthStatus};
use hnsx_core::chunk::{Artifact, Chunk};
use hnsx_core::error::{Error, Result};

const DEFAULT_TIMEOUT_SECONDS: u64 = 300;
const ANTHROPIC_BASE_URL: &str = "https://api.anthropic.com";
const ANTHROPIC_VERSION: &str = "2023-06-01";

pub struct AnthropicAdapter {
    client: reqwest::Client,
    api_key: String,
    model: String,
    system: String,
    base_url: String,
    timeout: Duration,
}

impl AnthropicAdapter {
    pub fn new(spec: &AgentSpec) -> Result<Self> {
        let api_key = std::env::var("ANTHROPIC_API_KEY")
            .map_err(|_| Error::Adapter("ANTHROPIC_API_KEY not set".into()))?;
        Self::new_with_key(spec, api_key)
    }

    pub fn new_with_key(spec: &AgentSpec, api_key: String) -> Result<Self> {
        let model = spec.model.model.clone();
        let system = spec.prompt.template.clone();
        let base_url = spec
            .model
            .endpoint
            .clone()
            .unwrap_or_else(|| ANTHROPIC_BASE_URL.into());
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

        if let Some(mem) = input.get("_memory").and_then(Value::as_array) {
            for m in mem {
                if let (Some(role), Some(content)) = (
                    m.get("role").and_then(Value::as_str),
                    m.get("content").and_then(Value::as_str),
                ) {
                    // Anthropic only supports user/assistant roles.
                    let role = if role == "system" { "user" } else { role };
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
impl Adapter for AnthropicAdapter {
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
            "system": self.system,
            "max_tokens": 4096,
            "stream": true,
        });

        let url = format!("{}/v1/messages", self.base_url.trim_end_matches('/'));
        let request = self
            .client
            .post(&url)
            .header("x-api-key", self.api_key.clone())
            .header("anthropic-version", ANTHROPIC_VERSION)
            .header("Content-Type", "application/json")
            .json(&body);

        let response = request
            .send()
            .await
            .map_err(|e| Error::Adapter(format!("Anthropic request failed: {e}")))?;

        let status = response.status();
        if !status.is_success() {
            let text = response.text().await.unwrap_or_default();
            return Err(classify_http_error(status, &text));
        }

        let model = self.model.clone();
        let system = self.system.clone();
        let sse_stream = parse_sse_stream(response.bytes_stream());

        Ok(Box::pin(stream! {
            let mut sse_stream = sse_stream;
            let mut prompt_tokens: u64 = 0;
            let mut completion_tokens: u64 = 0;

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

                match event.event.as_deref() {
                    Some("message_start") => {
                        if let Some(usage) = json.get("message").and_then(|m| m.get("usage")) {
                            prompt_tokens = usage.get("input_tokens").and_then(Value::as_u64).unwrap_or(prompt_tokens);
                            completion_tokens = usage.get("output_tokens").and_then(Value::as_u64).unwrap_or(completion_tokens);
                        }
                    }
                    Some("content_block_delta") => {
                        if let Some(text) = json
                            .get("delta")
                            .and_then(|d| d.get("text"))
                            .and_then(Value::as_str)
                        {
                            if !text.is_empty() {
                                yield Chunk::text(text);
                            }
                        }
                    }
                    Some("message_delta") => {
                        if let Some(usage) = json.get("usage") {
                            completion_tokens = usage.get("output_tokens").and_then(Value::as_u64).unwrap_or(completion_tokens);
                        }
                    }
                    _ => {}
                }
            }

            // Anthropic does not always report streaming usage; estimate if missing.
            if prompt_tokens == 0 {
                prompt_tokens = estimate_tokens(&system);
            }

            let cost_usd = crate::http_common::anthropic_cost(&model, prompt_tokens, completion_tokens);
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
            .header("x-api-key", self.api_key.clone())
            .header("anthropic-version", ANTHROPIC_VERSION)
            .timeout(Duration::from_secs(10))
            .send()
            .await
        {
            Ok(resp) if resp.status().is_success() => HealthStatus {
                healthy: true,
                message: Some(format!("Anthropic adapter for {}", self.model)),
            },
            Ok(resp) => HealthStatus {
                healthy: false,
                message: Some(format!("Anthropic health check failed: {}", resp.status())),
            },
            Err(e) => HealthStatus {
                healthy: false,
                message: Some(format!("Anthropic health check error: {e}")),
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
                provider: Provider::Anthropic,
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
    fn build_messages_maps_system_to_user() {
        let adapter = AnthropicAdapter::new_with_key(
            &spec("claude-3-haiku"), "test-key".into()).unwrap();
        let input = json!({
            "task": "hello",
            "_memory": [
                {"role": "system", "content": "be nice"},
                {"role": "user", "content": "previous"}
            ]
        });
        let messages = adapter.build_messages(&input);
        assert_eq!(messages.len(), 3);
        assert_eq!(messages[0]["role"], "user");
        assert_eq!(messages[0]["content"], "be nice");
        assert_eq!(messages[1]["role"], "user");
    }
}
