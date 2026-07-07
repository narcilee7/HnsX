//! Hermetic integration tests for the native HTTP adapters.
//!
//! These use `wiremock` to simulate OpenAI, Anthropic, Ollama, and custom
//! OpenAI-compatible endpoints, verifying streaming text + token usage
//! artifacts without real network calls.

use futures::StreamExt;
use serde_json::json;
use wiremock::matchers::{header, method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use hnsx_adapter::{
    AnthropicAdapter, CustomAdapter, OllamaAdapter, OpenAIAdapter,
};
use hnsx_core::adapter::Adapter;
use hnsx_core::agent::{
    AdapterConfig, AgentSpec, ModelRef, PromptTemplate, Provider,
};
use hnsx_core::chunk::{Artifact, Chunk};

fn spec(provider: Provider, model: &str, endpoint: Option<String>) -> AgentSpec {
    AgentSpec {
        id: "a".into(),
        description: "x".into(),
        model: ModelRef {
            provider,
            model: model.into(),
            endpoint,
        },
        adapter: AdapterConfig {
            timeout_seconds: Some(5),
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

#[tokio::test]
async fn openai_adapter_streams_text_and_usage() {
    let server = MockServer::start().await;

    let sse_body = concat!(
        "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n",
        "data: {\"id\":\"2\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n",
        "data: {\"id\":\"3\",\"object\":\"chat.completion.chunk\",\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2},\"choices\":[]}\n\n",
        "data: [DONE]\n\n"
    );

    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .and(header("Authorization", "Bearer test-key"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/event-stream")
                .set_body_string(sse_body),
        )
        .mount(&server)
        .await;

    let adapter = OpenAIAdapter::new_with_key(
        &spec(Provider::Openai, "gpt-4o-mini", Some(server.uri())),
        "test-key".into(),
    )
    .unwrap()
    .with_client(reqwest::Client::new());
    let ctx = adapter.prepare(&json!({})).await.unwrap();
    let mut stream = adapter.invoke(&json!({"task": "say hi"}), &ctx).await.unwrap();

    let mut texts = Vec::new();
    let mut usage: Option<Artifact> = None;
    while let Some(chunk) = stream.next().await {
        match chunk {
            Chunk::Text(t) => texts.push(t),
            Chunk::Artifact(a @ Artifact::TokenUsage { .. }) => usage = Some(a),
            Chunk::Error(e) => panic!("unexpected error: {e}"),
            _ => {}
        }
    }

    assert_eq!(texts.join(""), "Hello world");
    let (prompt, completion, cost_usd) = match usage.unwrap() {
        Artifact::TokenUsage { prompt, completion, cost_usd } => (prompt, completion, cost_usd),
        other => panic!("expected TokenUsage, got {other:?}"),
    };
    assert_eq!(prompt, 10);
    assert_eq!(completion, 2);
    assert!(cost_usd > 0.0);
}

#[tokio::test]
async fn anthropic_adapter_streams_text_and_usage() {
    let server = MockServer::start().await;

    let sse_body = concat!(
        "event: message_start\n",
        "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":15,\"output_tokens\":0}}}\n\n",
        "event: content_block_start\n",
        "data: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
        "event: content_block_delta\n",
        "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi there\"}}\n\n",
        "event: message_delta\n",
        "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":2}}\n\n",
        "event: message_stop\n",
        "data: {\"type\":\"message_stop\"}\n\n"
    );

    Mock::given(method("POST"))
        .and(path("/v1/messages"))
        .and(header("x-api-key", "test-key"))
        .and(header("anthropic-version", "2023-06-01"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/event-stream")
                .set_body_string(sse_body),
        )
        .mount(&server)
        .await;

    let adapter = AnthropicAdapter::new_with_key(
        &spec(Provider::Anthropic, "claude-3-haiku", Some(server.uri())),
        "test-key".into(),
    )
    .unwrap()
    .with_client(reqwest::Client::new());
    let ctx = adapter.prepare(&json!({})).await.unwrap();
    let mut stream = adapter.invoke(&json!({"task": "say hi"}), &ctx).await.unwrap();

    let mut texts = Vec::new();
    let mut usage: Option<Artifact> = None;
    while let Some(chunk) = stream.next().await {
        match chunk {
            Chunk::Text(t) => texts.push(t),
            Chunk::Artifact(a @ Artifact::TokenUsage { .. }) => usage = Some(a),
            Chunk::Error(e) => panic!("unexpected error: {e}"),
            _ => {}
        }
    }

    assert_eq!(texts.join(""), "Hi there");
    let (prompt, completion) = match usage.unwrap() {
        Artifact::TokenUsage { prompt, completion, .. } => (prompt, completion),
        other => panic!("expected TokenUsage, got {other:?}"),
    };
    assert_eq!(prompt, 15);
    assert_eq!(completion, 2);
}

#[tokio::test]
async fn ollama_adapter_streams_text_and_usage() {
    let server = MockServer::start().await;

    let ndjson_body = concat!(
        "{\"model\":\"llama3.1\",\"message\":{\"role\":\"assistant\",\"content\":\"Hey\"},\"done\":false}\n",
        "{\"model\":\"llama3.1\",\"message\":{\"role\":\"assistant\",\"content\":\"!\"},\"done\":false}\n",
        "{\"model\":\"llama3.1\",\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true,\"prompt_eval_count\":20,\"eval_count\":2}\n"
    );

    Mock::given(method("POST"))
        .and(path("/api/chat"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "application/x-ndjson")
                .set_body_string(ndjson_body),
        )
        .mount(&server)
        .await;

    let adapter = OllamaAdapter::new(
        &spec(Provider::Ollama, "llama3.1", Some(server.uri())),
    )
    .unwrap()
    .with_client(reqwest::Client::new());
    let ctx = adapter.prepare(&json!({})).await.unwrap();
    let mut stream = adapter.invoke(&json!({"task": "say hi"}), &ctx).await.unwrap();

    let mut texts = Vec::new();
    let mut usage: Option<Artifact> = None;
    while let Some(chunk) = stream.next().await {
        match chunk {
            Chunk::Text(t) => texts.push(t),
            Chunk::Artifact(a @ Artifact::TokenUsage { .. }) => usage = Some(a),
            Chunk::Error(e) => panic!("unexpected error: {e}"),
            _ => {}
        }
    }

    assert_eq!(texts.join(""), "Hey!");
    let (prompt, completion, cost_usd) = match usage.unwrap() {
        Artifact::TokenUsage { prompt, completion, cost_usd } => (prompt, completion, cost_usd),
        other => panic!("expected TokenUsage, got {other:?}"),
    };
    assert_eq!(prompt, 20);
    assert_eq!(completion, 2);
    assert_eq!(cost_usd, 0.0);
}

#[tokio::test]
async fn custom_adapter_streams_via_openai_compatible_endpoint() {
    let server = MockServer::start().await;

    let sse_body = concat!(
        "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\n",
        "data: {\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":1}}\n\n",
        "data: [DONE]\n\n"
    );

    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .and(header("Authorization", "Bearer custom-key"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/event-stream")
                .set_body_string(sse_body),
        )
        .mount(&server)
        .await;

    let adapter = CustomAdapter::new_with_key(
        &spec(Provider::Custom, "my-model", Some(server.uri())),
        "custom-key".into(),
    )
    .unwrap()
    .with_client(reqwest::Client::new());
    let ctx = adapter.prepare(&json!({})).await.unwrap();
    let mut stream = adapter.invoke(&json!({"task": "ok"}), &ctx).await.unwrap();

    let mut texts = Vec::new();
    let mut usage: Option<Artifact> = None;
    while let Some(chunk) = stream.next().await {
        match chunk {
            Chunk::Text(t) => texts.push(t),
            Chunk::Artifact(a @ Artifact::TokenUsage { .. }) => usage = Some(a),
            Chunk::Error(e) => panic!("unexpected error: {e}"),
            _ => {}
        }
    }

    assert_eq!(texts.join(""), "OK");
    let (prompt, completion) = match usage.unwrap() {
        Artifact::TokenUsage { prompt, completion, .. } => (prompt, completion),
        other => panic!("expected TokenUsage, got {other:?}"),
    };
    assert_eq!(prompt, 5);
    assert_eq!(completion, 1);
}

#[tokio::test]
async fn openai_adapter_classifies_http_errors() {
    let server = MockServer::start().await;

    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .respond_with(ResponseTemplate::new(401).set_body_string("invalid key"))
        .mount(&server)
        .await;

    let adapter = OpenAIAdapter::new_with_key(
        &spec(Provider::Openai, "gpt-4o-mini", Some(server.uri())),
        "bad-key".into(),
    )
    .unwrap()
    .with_client(reqwest::Client::new());
    let ctx = adapter.prepare(&json!({})).await.unwrap();
    let err = match adapter.invoke(&json!({"task": "x"}), &ctx).await {
        Ok(_) => panic!("expected error"),
        Err(e) => e,
    };
    let msg = format!("{err}");
    assert!(msg.contains("authentication failed"), "msg={msg}");
}
