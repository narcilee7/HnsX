//! Native OpenAI API adapter.
//!
//! Implements the `Adapter` trait directly over `reqwest` + SSE, providing
//! real token/cost reporting, provider-specific error classification, and
//! native tool-call support.

use std::collections::BTreeMap;
use std::time::Duration;

use async_stream::stream;
use async_trait::async_trait;
use futures::stream::{BoxStream, StreamExt};
use serde_json::Value;

use crate::http_common::{
    bearer, classify_http_error, openai_cost, parse_sse_stream, value_to_string,
};
use crate::tool_chat::{execute_tool, tool_definitions, MAX_TOOL_ROUNDS, PartialToolCall};
use hnsx_core::adapter::{Adapter, RuntimeContext};
use hnsx_core::agent::{AgentSpec, HealthStatus};
use hnsx_core::chunk::{Artifact, Chunk};
use hnsx_core::error::{Error, Result};
use hnsx_core::tool::ToolRegistry;

const DEFAULT_TIMEOUT_SECONDS: u64 = 300;
const OPENAI_BASE_URL: &str = "https://api.openai.com";

pub struct OpenAIAdapter {
    client: reqwest::Client,
    api_key: String,
    model: String,
    system: String,
    base_url: String,
    timeout: Duration,
    tools: Option<ToolRegistry>,
    tool_defs: Option<Value>,
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

    async fn invoke_with_tools(
        &self,
        input: &Value,
    ) -> Result<BoxStream<'static, Chunk>> {
        let mut messages = self.build_messages(input);
        let tool_defs = self.tool_defs.clone().expect("invoke_with_tools requires tools");
        let registry = self.tools.clone().expect("invoke_with_tools requires tools");
        let client = self.client.clone();
        let api_key = self.api_key.clone();
        let model = self.model.clone();
        let url = format!("{}/v1/chat/completions", self.base_url.trim_end_matches('/'));

        Ok(Box::pin(stream! {
            let mut prompt_tokens: u64 = 0;
            let mut completion_tokens: u64 = 0;
            let mut final_text = String::new();

            for _round in 0..=MAX_TOOL_ROUNDS {
                let body = serde_json::json!({
                    "model": model,
                    "messages": messages,
                    "stream": true,
                    "stream_options": { "include_usage": true },
                    "tools": tool_defs,
                });

                let request = client
                    .post(&url)
                    .header("Authorization", bearer(&api_key))
                    .header("Content-Type", "application/json")
                    .json(&body);

                let response = match request.send().await {
                    Ok(r) => r,
                    Err(e) => {
                        yield Chunk::error(format!("OpenAI request failed: {e}"));
                        return;
                    }
                };

                let status = response.status();
                if !status.is_success() {
                    let text = response.text().await.unwrap_or_default();
                    yield Chunk::error(classify_http_error(status, &text).to_string());
                    return;
                }

                let mut sse_stream = parse_sse_stream(response.bytes_stream());
                let mut partials: BTreeMap<u32, PartialToolCall> = BTreeMap::new();
                let mut assistant_text = String::new();
                let mut finish_reason: Option<String> = None;

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
                                    assistant_text.push_str(text);
                                }
                                if let Some(tcs) = delta.get("tool_calls").and_then(Value::as_array) {
                                    for tc in tcs {
                                        let index = tc.get("index").and_then(Value::as_u64).unwrap_or(0) as u32;
                                        let entry = partials.entry(index).or_default();
                                        if let Some(id) = tc.get("id").and_then(Value::as_str) {
                                            entry.id = id.to_string();
                                        }
                                        if let Some(fn_obj) = tc.get("function") {
                                            if let Some(name) = fn_obj.get("name").and_then(Value::as_str) {
                                                entry.name.push_str(name);
                                            }
                                            if let Some(args) = fn_obj.get("arguments").and_then(Value::as_str) {
                                                entry.arguments.push_str(args);
                                            }
                                        }
                                    }
                                }
                            }
                            if let Some(reason) = choice.get("finish_reason").and_then(Value::as_str) {
                                finish_reason = Some(reason.to_string());
                            }
                        }
                    }
                }

                if finish_reason.as_deref() == Some("tool_calls") || !partials.is_empty() {
                    messages.push(assistant_tool_message(&assistant_text, &partials));
                    for (index, tc) in &partials {
                        let id = if tc.id.is_empty() {
                            format!("call_{index}")
                        } else {
                            tc.id.clone()
                        };
                        let args = parse_arguments(&tc.arguments);
                        let result = execute_tool(&registry, &tc.name, args).await;
                        messages.push(tool_message(&id, result));
                    }
                    continue;
                }

                final_text = assistant_text;
                break;
            }

            if !final_text.is_empty() {
                yield Chunk::text(final_text);
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
}

#[async_trait]
impl Adapter for OpenAIAdapter {
    async fn prepare(&self, _config: &Value) -> Result<RuntimeContext> {
        Ok(RuntimeContext {
            session_id: uuid::Uuid::new_v4().to_string(),
            config: Value::Null,
        })
    }

    async fn invoke(&self,
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

fn json_message(role: &str, content: Value) -> Value {
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

fn assistant_tool_message(content: &str, tool_calls: &BTreeMap<u32, PartialToolCall>) -> Value {
    let calls: Vec<Value> = tool_calls
        .iter()
        .map(|(index, tc)| {
            let id = if tc.id.is_empty() {
                format!("call_{index}")
            } else {
                tc.id.clone()
            };
            serde_json::json!({
                "id": id,
                "type": "function",
                "function": {
                    "name": tc.name,
                    "arguments": tc.arguments,
                }
            })
        })
        .collect();

    serde_json::json!({
        "role": "assistant",
        "content": if content.is_empty() { Value::Null } else { Value::String(content.to_string()) },
        "tool_calls": calls,
    })
}

fn tool_message(id: &str, content: Value) -> Value {
    let text = serde_json::to_string(&content)
        .unwrap_or_else(|e| format!("{{\"error\":\"{e}\"}}"));
    serde_json::json!({
        "role": "tool",
        "tool_call_id": id,
        "content": text,
    })
}

fn parse_arguments(s: &str) -> Value {
    let trimmed = s.trim();
    if trimmed.is_empty() {
        Value::Null
    } else {
        serde_json::from_str(trimmed).unwrap_or_else(|_| Value::String(trimmed.to_string()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use hnsx_core::agent::{AdapterConfig, ModelRef, PromptTemplate, Provider, ToolKind, ToolRef};
    use serde_json::json;
    use wiremock::matchers::{body_string_contains, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    fn spec(model: &str) -> AgentSpec {
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

    fn spec_with_tools(model: &str, endpoint: &str, tools: Vec<ToolRef>) -> AgentSpec {
        AgentSpec {
            id: "a".into(),
            description: "x".into(),
            model: ModelRef {
                provider: Provider::Openai,
                model: model.into(),
                endpoint: Some(endpoint.into()),
            },
            adapter: AdapterConfig {
                timeout_seconds: Some(30),
                extra: json!({}),
            },
            tools,
            prompt: PromptTemplate {
                template: "You are a test assistant.".into(),
                variables: json!({}),
            },
            sandbox: None,
            memory_window: None,
        }
    }

    fn sse_event(obj: &str) -> String {
        format!("data: {obj}\n\n")
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

    #[tokio::test(flavor = "multi_thread")]
    async fn tool_call_loop_executes_http_tool_and_returns_final_answer() {
        let api_server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/data"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({"items": [1, 2]})))
            .expect(1)
            .mount(&api_server)
            .await;

        let llm_server = MockServer::start().await;

        let tool_call_sse = sse_event(r#"{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":null},"finish_reason":null}]}"#)
            + &sse_event(r#"{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"api","arguments":""}}]},"finish_reason":null}]}"#)
            + &sse_event(r#"{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"/data\"}"}}]},"finish_reason":null}]}"#)
            + &sse_event(r#"{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}"#)
            + "data: [DONE]\n\n";

        let final_sse = sse_event(r#"{"id":"chatcmpl-2","object":"chat.completion.chunk","created":2,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"Done"},"finish_reason":null}]}"#)
            + &sse_event(r#"{"id":"chatcmpl-2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}"#)
            + &sse_event(r#"{"id":"chatcmpl-2","object":"chat.completion.chunk","usage":{"prompt_tokens":10,"completion_tokens":1}}"#)
            + "data: [DONE]\n\n";

        // Register the final-answer mock first: it only matches once tool
        // results have been added to the conversation.
        Mock::given(method("POST"))
            .and(path("/v1/chat/completions"))
            .and(body_string_contains("\"role\":\"tool\""))
            .respond_with(ResponseTemplate::new(200).set_body_string(final_sse))
            .expect(1)
            .mount(&llm_server)
            .await;

        Mock::given(method("POST"))
            .and(path("/v1/chat/completions"))
            .and(body_string_contains("tools"))
            .respond_with(ResponseTemplate::new(200).set_body_string(tool_call_sse))
            .expect(1)
            .mount(&llm_server)
            .await;

        let tools = vec![ToolRef {
            kind: ToolKind::Http,
            name: "api".into(),
            config: json!({"base_url": api_server.uri()}),
        }];
        let spec = spec_with_tools("gpt-4o-mini", &llm_server.uri(), tools);
        let registry = crate::tools::build_tool_registry(&spec.tools).expect("build registry");
        let adapter = OpenAIAdapter::new_with_key(&spec, "test-key".into())
            .unwrap()
            .with_tools(registry);

        let ctx = adapter.prepare(&json!({})).await.unwrap();
        let mut stream = adapter.invoke(&json!("call the api"), &ctx).await.unwrap();

        let mut texts = Vec::new();
        let mut usage = None;
        while let Some(chunk) = stream.next().await {
            match chunk {
                Chunk::Text(t) => texts.push(t),
                Chunk::Artifact(Artifact::TokenUsage { prompt, completion, cost_usd }) => {
                    usage = Some((prompt, completion, cost_usd));
                }
                Chunk::Error(e) => panic!("unexpected error chunk: {e}"),
                _ => {}
            }
        }

        assert_eq!(texts.join(""), "Done");
        assert!(usage.is_some(), "expected token usage artifact");
    }

    #[tokio::test(flavor = "multi_thread")]
    async fn tool_call_unknown_tool_yields_error() {
        let llm_server = MockServer::start().await;
        let tool_call_sse = sse_event(r#"{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_x","type":"function","function":{"name":"missing","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}"#)
            + "data: [DONE]\n\n";

        Mock::given(method("POST"))
            .and(path("/v1/chat/completions"))
            .respond_with(ResponseTemplate::new(200).set_body_string(tool_call_sse))
            .up_to_n_times(1)
            .expect(1)
            .mount(&llm_server)
            .await;

        let tools = vec![ToolRef {
            kind: ToolKind::Http,
            name: "api".into(),
            config: json!({"base_url": "http://localhost"}),
        }];
        let spec = spec_with_tools("gpt-4o-mini", &llm_server.uri(), tools);
        let registry = crate::tools::build_tool_registry(&spec.tools).expect("build registry");
        let adapter = OpenAIAdapter::new_with_key(&spec, "test-key".into())
            .unwrap()
            .with_tools(registry);

        let ctx = adapter.prepare(&json!({})).await.unwrap();
        let mut stream = adapter.invoke(&json!("test"), &ctx).await.unwrap();

        let mut saw_error = false;
        while let Some(chunk) = stream.next().await {
            if let Chunk::Error(_) = chunk {
                saw_error = true;
                break;
            }
        }
        assert!(saw_error, "expected an error chunk for unknown tool");
    }
}
