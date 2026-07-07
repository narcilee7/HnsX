//! Integration tests for memory-aware workflow execution.

use futures::StreamExt;
use hnsx_core::DomainLoader;
use hnsx_core::chunk::Chunk;
use hnsx_core::memory::{InMemoryBackend, MemoryBackend};
use serde_json::json;

const MEMORY_YAML: &str = r#"
id: test-memory
version: 0.1.0
description: Two-step noop pipeline with memory.
agents:
  - id: greeter
    description: greeter
    model: { provider: openai, model: gpt-4o-mini }
    adapter: {}
    prompt: { template: "You are a greeter.", variables: {} }
    memory_window: 5
  - id: responder
    description: responder
    model: { provider: openai, model: gpt-4o-mini }
    adapter: {}
    prompt: { template: "You are a responder.", variables: {} }
    memory_window: 5
workflow:
  entry: greet
  steps:
    - id: greet
      agent: greeter
      output: greeting
    - id: respond
      agent: responder
      input:
        greeting: "${steps.greet.output}"
"#;

fn collect(stream: impl futures::Stream<Item = Chunk> + Unpin) -> Vec<Chunk> {
    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_time()
        .build()
        .expect("tokio runtime");
    rt.block_on(async {
        let mut s = stream;
        let mut out = Vec::new();
        while let Some(c) = s.next().await {
            out.push(c);
        }
        out
    })
}

#[test]
fn workflow_injects_memory_context() {
    let memory = std::sync::Arc::new(InMemoryBackend::new());
    let loader = DomainLoader::with_factory(std::sync::Arc::new(hnsx_core::NoopFactory))
        .with_memory(memory.clone())
        .with_memory_window(5);

    let domain = loader.from_str(MEMORY_YAML).expect("should load");
    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_time()
        .build()
        .expect("tokio runtime");

    // First invocation: seed memory with a greeting.
    let stream1 = rt
        .block_on(domain.invoke(json!({"session_id": "s1", "name": "Alice"})))
        .expect("invoke");
    let chunks1 = collect(Box::pin(stream1));
    assert!(matches!(chunks1.last(), Some(Chunk::Done { .. })));

    // Verify turns were saved.
    let session = rt
        .block_on(memory.load_session("test-memory", "s1"))
        .expect("load session");
    assert_eq!(session.turns.len(), 2, "expected 2 assistant turns");
    assert_eq!(session.turns[0].agent_id, "greeter");
    assert_eq!(session.turns[1].agent_id, "responder");

    // Second invocation with the same session: both agents should see prior context.
    let stream2 = rt
        .block_on(domain.invoke(json!({"session_id": "s1", "name": "Alice"})))
        .expect("invoke");
    let chunks2 = collect(Box::pin(stream2));
    let done = chunks2.last().expect("done");
    let vars = match done {
        Chunk::Done { variables } => variables,
        _ => panic!(),
    };
    assert!(vars.get("steps.greet.output").is_some());
    assert!(vars.get("steps.respond.output").is_some());
}

#[test]
fn sqlite_memory_persists_across_invocations() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("sessions.db");
    let config = hnsx_core::memory::MemoryConfig::Structured {
        backend: "sqlite".into(),
        options: json!({"path": path.to_string_lossy().to_string()}),
    };
    let memory =
        hnsx_core::memory::MemoryBackendFactory::create(&config).expect("create sqlite backend");

    let loader = DomainLoader::with_factory(std::sync::Arc::new(hnsx_core::NoopFactory))
        .with_memory(memory.clone())
        .with_memory_window(5);

    let domain = loader.from_str(MEMORY_YAML).expect("should load");
    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_time()
        .build()
        .expect("tokio runtime");

    let stream = rt
        .block_on(domain.invoke(json!({"session_id": "s2", "name": "Bob"})))
        .expect("invoke");
    let _chunks = collect(Box::pin(stream));

    // Re-open the same SQLite file to simulate a fresh process.
    let memory2 =
        hnsx_core::memory::MemoryBackendFactory::create(&config).expect("create sqlite backend");
    let session = rt
        .block_on(memory2.load_session("test-memory", "s2"))
        .expect("load session");
    assert_eq!(session.turns.len(), 2);
}
