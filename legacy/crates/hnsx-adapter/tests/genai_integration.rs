//! Hermetic integration tests for `GenaiAgent` using `wiremock` to stand
//! in for the OpenAI HTTP endpoint. No network, no API keys needed.

use std::sync::Arc;

use futures::StreamExt;
use genai::adapter::AdapterKind;
use genai::resolver::{AuthData, Endpoint};
use genai::{Client, ServiceTarget};
use hnsx_adapter::GenaiAgent;
use hnsx_core::agent::{Agent, InvokeContext};
use hnsx_core::chunk::Chunk;
use hnsx_core::tool::ToolRegistry;
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

/// Build a genai `Client` that redirects any `openai::*` or `anthropic::*`
/// model to `base_url`. The two providers share the same wiremock server
/// in tests; the URL path (`/v1/chat/completions` vs `/v1/messages`) is
/// decided by genai based on the adapter kind.
fn client_pointing_at(base_url: String) -> Client {
    Client::builder()
        .with_service_target_resolver_fn(move |mut target: ServiceTarget| {
            match target.model.adapter_kind {
                AdapterKind::OpenAI | AdapterKind::Anthropic => {
                    target.endpoint = Endpoint::from_owned(base_url.clone());
                    target.auth = AuthData::from_single("sk-test");
                }
                _ => {}
            }
            Ok(target)
        })
        .build()
}

const SSE_HELLO: &str = "\
data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n\
data: [DONE]\n\n";

/// Anthropic Messages API SSE format. `genai` parses the event types and
/// flattens `content_block_delta.delta.text` chunks into a text stream.
const ANTHROPIC_SSE_HELLO: &str = "\
event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-haiku-4-5\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n\
event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n\
event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n\
event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n\
event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n\
event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":2}}\n\n\
event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n";

#[tokio::test(flavor = "multi_thread")]
async fn genai_agent_streams_text_chunks() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/event-stream")
                .set_body_string(SSE_HELLO),
        )
        .expect(1)
        .mount(&server)
        .await;

    let client = client_pointing_at(format!("{}/v1/", server.uri()));
    let agent = GenaiAgent::new(
        client,
        "openai::gpt-4o-mini".into(),
        "you are a test".into(),
        ToolRegistry::new(),
    );

    let mut stream = agent
        .invoke(
            json!({"q": "hi"}),
            InvokeContext {
                session_id: "s1".into(),
                domain_id: "d1".into(),
                agent_id: "a".into(),
            },
        )
        .await
        .expect("invoke should succeed");

    let mut texts: Vec<String> = Vec::new();
    let mut errors: Vec<String> = Vec::new();
    while let Some(chunk) = stream.next().await {
        match chunk {
            Chunk::Text(t) => texts.push(t),
            Chunk::Error(e) => errors.push(e),
            _ => {}
        }
    }
    assert!(errors.is_empty(), "unexpected errors: {errors:?}");
    assert_eq!(texts.join(""), "Hello world");
}

#[tokio::test(flavor = "multi_thread")]
async fn genai_agent_streams_anthropic_response() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/messages"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/event-stream")
                .set_body_string(ANTHROPIC_SSE_HELLO),
        )
        .expect(1)
        .mount(&server)
        .await;

    let client = client_pointing_at(format!("{}/v1/", server.uri()));
    let agent = GenaiAgent::new(
        client,
        "anthropic::claude-haiku-4-5".into(),
        "you are a test".into(),
        ToolRegistry::new(),
    );

    let mut stream = agent
        .invoke(
            json!({"q": "hi"}),
            InvokeContext {
                session_id: "s1".into(),
                domain_id: "d1".into(),
                agent_id: "a".into(),
            },
        )
        .await
        .expect("invoke should succeed");

    let mut texts: Vec<String> = Vec::new();
    let mut errors: Vec<String> = Vec::new();
    while let Some(chunk) = stream.next().await {
        match chunk {
            Chunk::Text(t) => texts.push(t),
            Chunk::Error(e) => errors.push(e),
            _ => {}
        }
    }
    assert!(errors.is_empty(), "unexpected errors: {errors:?}");
    assert_eq!(
        texts.join(""),
        "Hello world",
        "Anthropic delta.text chunks should be joined in order"
    );
}

#[tokio::test(flavor = "multi_thread")]
async fn genai_agent_surfaces_error_on_http_failure() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .respond_with(ResponseTemplate::new(401))
        .mount(&server)
        .await;

    let client = client_pointing_at(format!("{}/v1/", server.uri()));
    let agent = GenaiAgent::new(
        client,
        "openai::gpt-4o-mini".into(),
        "test".into(),
        ToolRegistry::new(),
    );

    // genai's openai adapter surfaces HTTP errors either at the first
    // await (return Err from exec_chat_stream) or as an error event in
    // the stream. We accept either: invoke may fail, or it succeeds but
    // the stream yields a Chunk::Error.
    let result = agent
        .invoke(
            json!({}),
            InvokeContext {
                session_id: String::new(),
                domain_id: String::new(),
                agent_id: String::new(),
            },
        )
        .await;

    let mut saw_error = result.is_err();
    if let Ok(mut stream) = result {
        while let Some(chunk) = stream.next().await {
            if let Chunk::Error(_) = chunk {
                saw_error = true;
            }
        }
    }
    assert!(saw_error, "expected an error to surface on 401");
}

// Hint to the linker that `Arc` is used (the unused-import warning fires
// if I switch to a different test runner); keeps the lints clean.
#[allow(dead_code)]
fn _force_arc(a: Arc<()>) {
    drop(a);
}
