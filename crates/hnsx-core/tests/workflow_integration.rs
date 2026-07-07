//! End-to-end workflow tests: load a domain via `DomainLoader`, invoke it,
//! and assert on the resulting chunk stream.
//!
//! Phase 1.2: agents are noop stubs. Real adapters land in Phase 1.4+.

use std::path::PathBuf;

use futures::StreamExt;
use hnsx_core::DomainLoader;
use hnsx_core::chunk::Chunk;
use serde_json::json;

const PIPELINE_YAML: &str = r#"
id: test-pipeline
version: 0.1.0
description: Two-step noop pipeline.
agents:
  - id: a
    description: first
    model: { provider: openai, model: gpt-4o-mini }
    adapter: {}
    prompt: { template: t, variables: {} }
  - id: b
    description: second
    model: { provider: openai, model: gpt-4o-mini }
    adapter: {}
    prompt: { template: t, variables: {} }
workflow:
  entry: s1
  steps:
    - id: s1
      agent: a
    - id: s2
      agent: b
      input:
        prev: "${steps.s1.output}"
"#;

fn workspace_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(|p| p.parent())
        .expect("workspace root")
        .to_path_buf()
}

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

fn run_inline(yaml: &str, trigger: serde_json::Value) -> Vec<Chunk> {
    let domain = DomainLoader::new().from_str(yaml).expect("should load");
    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_time()
        .build()
        .expect("tokio runtime");
    let stream = rt.block_on(domain.invoke(trigger)).expect("invoke");
    collect(Box::pin(stream))
}

#[test]
fn pipeline_runs_two_steps_and_chains_output() {
    let chunks = run_inline(PIPELINE_YAML, json!({}));
    let text_count = chunks
        .iter()
        .filter(|c| matches!(c, Chunk::Text(_)))
        .count();
    assert_eq!(text_count, 2, "expected 2 text chunks, got: {chunks:?}");
    assert!(matches!(chunks.last(), Some(Chunk::Done { .. })));

    let done = chunks.last().expect("done");
    let vars = match done {
        Chunk::Done { variables } => variables,
        _ => panic!(),
    };
    let s1 = vars
        .get("steps.s1.output")
        .and_then(|v| v.as_str())
        .unwrap();
    let s2 = vars
        .get("steps.s2.output")
        .and_then(|v| v.as_str())
        .unwrap();
    assert!(
        s2.contains(s1),
        "s2 input should embed s1 output; s1={s1} s2={s2}"
    );
}

#[test]
fn example_customer_service_runs_end_to_end() {
    let path = workspace_root()
        .join("domains")
        .join("customer-service")
        .join("domain.yaml");
    let domain = DomainLoader::new()
        .from_path(&path)
        .expect("should load customer-service");
    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_time()
        .build()
        .expect("tokio runtime");
    let stream = rt.block_on(domain.invoke(json!({"message": "test"}))).expect("invoke");
    let chunks = collect(Box::pin(stream));

    let text_count = chunks
        .iter()
        .filter(|c| matches!(c, Chunk::Text(_)))
        .count();
    assert_eq!(
        text_count, 2,
        "expected 2 text chunks for 2 steps, got: {chunks:?}"
    );

    let done = chunks.last().expect("done");
    let vars = match done {
        Chunk::Done { variables } => variables,
        _ => panic!(),
    };
    assert!(vars.get("steps.classify.output").is_some());
    assert!(vars.get("steps.route.output").is_some());
}
