//! End-to-end runtime test: load the real `domains/customer-service/domain.yaml`
//! through `DomainLoader` + `HnsxAgentFactory`, mock both LLM endpoints with
//! `wiremock`, and assert the full workflow produces a final answer while
//! writing traces and persisting memory.

use std::path::PathBuf;
use std::sync::Arc;

use futures::StreamExt;
use serde_json::json;
use wiremock::matchers::{header, method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use hnsx_adapter::HnsxAgentFactory;
use hnsx_core::DomainLoader;
use hnsx_core::agent_factory::AgentFactory;
use hnsx_core::chunk::Chunk;
use hnsx_core::memory::{MemoryBackendFactory, MemoryConfig};
use hnsx_core::telemetry::Telemetry;
use hnsx_sandbox::factory::SandboxFactory;

fn workspace_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(|p| p.parent())
        .expect("workspace root")
        .to_path_buf()
}

fn customer_service_path() -> PathBuf {
    workspace_root()
        .join("domains")
        .join("customer-service")
        .join("domain.yaml")
}

/// OpenAI-style SSE that ends with a usage block.
const OPENAI_SSE: &str = concat!(
    "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"billing\"}}]}\n\n",
    "data: {\"id\":\"2\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"\"}}]}\n\n",
    "data: {\"id\":\"3\",\"object\":\"chat.completion.chunk\",\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":1},\"choices\":[]}\n\n",
    "data: [DONE]\n\n"
);

/// Anthropic-style SSE for the triage agent.
const ANTHROPIC_SSE: &str = concat!(
    "event: message_start\n",
    "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":12,\"output_tokens\":0}}}\n\n",
    "event: content_block_delta\n",
    "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"billing\"}}\n\n",
    "event: message_delta\n",
    "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":1}}\n\n",
    "event: message_stop\n",
    "data: {\"type\":\"message_stop\"}\n\n"
);

fn build_factory() -> Arc<dyn AgentFactory> {
    Arc::new(HnsxAgentFactory::with_sandbox_factory(Arc::new(
        SandboxFactory::new(),
    )))
}

#[tokio::test(flavor = "multi_thread")]
async fn customer_service_runs_with_mocked_llms() {
    let server = MockServer::start().await;

    // Both agents point to the same wiremock server; paths distinguish OpenAI
    // vs Anthropic endpoints.
    Mock::given(method("POST"))
        .and(path("/v1/messages"))
        .and(header("x-api-key", "anthropic-key"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/event-stream")
                .set_body_string(ANTHROPIC_SSE),
        )
        .expect(1)
        .mount(&server)
        .await;

    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .and(header("Authorization", "Bearer openai-key"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/event-stream")
                .set_body_string(OPENAI_SSE),
        )
        .expect(1)
        .mount(&server)
        .await;

    unsafe {
        std::env::set_var("ANTHROPIC_API_KEY", "anthropic-key");
        std::env::set_var("OPENAI_API_KEY", "openai-key");
    }

    // Point both providers at the mock server by overriding their base URLs
    // via environment variables used by the adapters. The native adapters read
    // `spec.model.endpoint`; since we cannot edit the YAML, we patch the env
    // that the spec loader reads... actually the loader does not read env for
    // endpoints. Instead we temporarily rewrite the domain YAML in memory.
    let yaml = tokio::fs::read_to_string(customer_service_path())
        .await
        .expect("read domain.yaml");
    let yaml = yaml.replace(
        "provider: anthropic",
        &format!("provider: anthropic\n      endpoint: {}", server.uri()),
    );
    let yaml = yaml.replace(
        "provider: openai",
        &format!("provider: openai\n      endpoint: {}", server.uri()),
    );

    let factory = build_factory();
    let domain = DomainLoader::with_factory(factory)
        .from_str(&yaml)
        .expect("load customer-service domain");

    let mut stream = domain
        .invoke(json!({"question": "Why was I charged twice?"}))
        .await
        .expect("invoke domain");

    let mut texts = Vec::new();
    let mut done_vars = None;
    while let Some(chunk) = stream.next().await {
        match chunk {
            Chunk::Text(t) => texts.push(t),
            Chunk::Done { variables } => done_vars = Some(variables),
            Chunk::Error(e) => panic!("unexpected error: {e}"),
            _ => {}
        }
    }

    assert!(!texts.is_empty(), "expected at least one text chunk");
    let vars = done_vars.expect("expected final Done chunk");
    assert!(vars.get("steps.classify.output").is_some());
    assert!(vars.get("steps.route.output").is_some());
}

#[tokio::test(flavor = "multi_thread")]
async fn customer_service_writes_traces_and_sqlite_memory() {
    let server = MockServer::start().await;

    Mock::given(method("POST"))
        .and(path("/v1/messages"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/event-stream")
                .set_body_string(ANTHROPIC_SSE),
        )
        .mount(&server)
        .await;

    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/event-stream")
                .set_body_string(OPENAI_SSE),
        )
        .mount(&server)
        .await;

    unsafe {
        std::env::set_var("ANTHROPIC_API_KEY", "anthropic-key");
        std::env::set_var("OPENAI_API_KEY", "openai-key");
    }

    let yaml = tokio::fs::read_to_string(customer_service_path())
        .await
        .expect("read domain.yaml");
    let yaml = yaml.replace(
        "provider: anthropic",
        &format!("provider: anthropic\n      endpoint: {}", server.uri()),
    );
    let yaml = yaml.replace(
        "provider: openai",
        &format!("provider: openai\n      endpoint: {}", server.uri()),
    );

    // Add a sqlite memory backend to the domain.
    let db_path = workspace_root()
        .join("target")
        .join("tmp")
        .join("e2e_sessions.db");
    let _ = tokio::fs::remove_file(&db_path).await;
    let yaml = yaml.replace(
        "workflow:\n",
        &format!(
            "memory:\n  backend: sqlite\n  path: {}\nworkflow:\n",
            db_path.to_string_lossy()
        ),
    );

    let trace_dir = workspace_root()
        .join("target")
        .join("tmp")
        .join("e2e_traces");
    let _ = tokio::fs::remove_dir_all(&trace_dir).await;
    tokio::fs::create_dir_all(&trace_dir)
        .await
        .expect("create trace dir");
    let telemetry = Arc::new(Telemetry::with_dir(trace_dir.clone()).expect("create telemetry dir"));

    let factory = build_factory();
    let memory = MemoryBackendFactory::create(&MemoryConfig::Structured {
        backend: "sqlite".into(),
        options: json!({"path": db_path.to_string_lossy().to_string()}),
    })
    .expect("create sqlite memory");

    let domain = DomainLoader::with_factory(factory)
        .with_telemetry(telemetry.clone())
        .with_memory(memory.clone())
        .from_str(&yaml)
        .expect("load domain with memory");

    let trigger = json!({"question": "Why was I charged twice?", "session_id": "s-e2e-1"});
    let mut stream = domain.invoke(trigger.clone()).await.expect("invoke");
    while stream.next().await.is_some() {}

    // Verify trace file was written.
    let trace_file = trace_dir.join("s-e2e-1.jsonl");
    assert!(trace_file.exists(), "trace file should exist");
    let contents = tokio::fs::read_to_string(&trace_file)
        .await
        .expect("read trace");
    assert!(!contents.is_empty(), "trace file should not be empty");
    let lines: Vec<&str> = contents.lines().collect();
    assert_eq!(lines.len(), 2, "expected one trace line per step");

    // Verify sqlite memory persisted turns for both agents.
    let session = memory
        .load_session("customer-service", "s-e2e-1")
        .await
        .expect("load session");
    assert_eq!(session.turns.len(), 2, "expected 2 assistant turns");
}
