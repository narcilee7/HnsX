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
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

/// Build a genai `Client` that redirects any `openai::*` model to `base_url`.
fn client_pointing_at(base_url: String) -> Client {
    Client::builder()
        .with_service_target_resolver_fn(move |mut target: ServiceTarget| {
            if target.model.adapter_kind == AdapterKind::OpenAI {
                target.endpoint = Endpoint::from_owned(base_url.clone());
                target.auth = AuthData::from_single("sk-test");
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
async fn genai_agent_surfaces_error_on_http_failure() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .respond_with(ResponseTemplate::new(401))
        .mount(&server)
        .await;

    let client = client_pointing_at(format!("{}/v1/", server.uri()));
    let agent = GenaiAgent::new(client, "openai::gpt-4o-mini".into(), "test".into());

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
