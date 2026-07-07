//! Hermetic integration test for the Ollama adapter via `genai`.
//!
//! `genai` maps `ollama::*` models to its Ollama adapter, which speaks the
//! Ollama `/api/chat` endpoint. We mock that endpoint with wiremock and
//! redirect the adapter to it through a custom `ServiceTargetResolver`.

use futures::StreamExt;
use genai::Client;
use genai::ServiceTarget;
use genai::adapter::AdapterKind;
use genai::resolver::{AuthData, Endpoint, ServiceTargetResolver};
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use hnsx_adapter::GenaiAgentFactory;
use hnsx_core::agent::{AgentSpec, InvokeContext, ModelRef, PromptTemplate, Provider};
use hnsx_core::agent_factory::AgentFactory;
use hnsx_core::chunk::Chunk;

fn ollama_client(base_url: String) -> Client {
    // Ollama adapter concatenates the endpoint with "api/chat", so the base
    // URL must end with a slash.
    let base_url = base_url.trim_end_matches('/').to_string() + "/";
    Client::builder()
        .with_service_target_resolver(ServiceTargetResolver::from_resolver_fn(
            move |mut target: ServiceTarget| {
                if target.model.adapter_kind == AdapterKind::Ollama {
                    target.endpoint = Endpoint::from_owned(base_url.clone());
                    target.auth = AuthData::None;
                }
                Ok(target)
            },
        ))
        .build()
}

fn spec() -> AgentSpec {
    AgentSpec {
        id: "chat".into(),
        description: "x".into(),
        model: ModelRef {
            provider: Provider::Ollama,
            model: "llama3.1".into(),
            endpoint: None,
        },
        adapter: hnsx_core::agent::AdapterConfig {
            timeout_seconds: None,
            extra: json!({}),
        },
        tools: vec![],
        prompt: PromptTemplate {
            template: "you are a test".into(),
            variables: json!({}),
        },
        sandbox: None,
    }
}

#[tokio::test(flavor = "multi_thread")]
async fn genai_agent_streams_ollama_ndjson() {
    let server = MockServer::start().await;

    // Ollama /api/chat returns a stream of NDJSON objects, one per line.
    let body = r#"{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":"Hello"},"done":false}
{"model":"llama3.1","created_at":"2024-01-01T00:00:01Z","message":{"role":"assistant","content":" world"},"done":false}
{"model":"llama3.1","created_at":"2024-01-01T00:00:02Z","message":{"role":"assistant","content":""},"done":true}
"#;
    Mock::given(method("POST"))
        .and(path("/api/chat"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "application/x-ndjson")
                .set_body_string(body),
        )
        .expect(1)
        .mount(&server)
        .await;

    let factory = GenaiAgentFactory::with_client(ollama_client(server.uri()));
    let agent = factory.create(&spec()).expect("create agent");

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
