//! End-to-end telemetry test: load a domain with a tempdir telemetry sink,
//! invoke it, and assert the JSONL trace has one entry per step.

use std::sync::Arc;

use futures::StreamExt;
use hnsx_core::DomainLoader;
use hnsx_core::chunk::Chunk;
use hnsx_core::telemetry::Telemetry;
use serde_json::json;
use tempfile::tempdir;

const PIPELINE_YAML: &str = r#"
id: test-pipeline
version: 0.1.0
description: Two-step pipeline for telemetry end-to-end test.
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

#[test]
fn workflow_emits_one_trace_line_per_step() {
    let dir = tempdir().expect("tempdir");
    let telemetry = Arc::new(Telemetry::with_dir(dir.path().to_path_buf()).expect("with_dir"));

    let domain = DomainLoader::with_factory(Arc::new(hnsx_core::NoopFactory))
        .with_telemetry(telemetry)
        .from_str(PIPELINE_YAML)
        .expect("load");

    // Drain the stream.
    let stream = futures::executor::block_on(domain.invoke(json!({}))).expect("invoke");
    let mut s = Box::pin(stream);
    while let Some(chunk) = futures::executor::block_on(s.next()) {
        if let Chunk::Error(e) = chunk {
            panic!("unexpected error: {e}");
        }
    }

    // Find the trace file (single session -> single file).
    let entries: Vec<_> = std::fs::read_dir(dir.path())
        .expect("read_dir")
        .filter_map(Result::ok)
        .filter(|e| e.path().extension().map(|x| x == "jsonl").unwrap_or(false))
        .collect();
    assert_eq!(entries.len(), 1, "expected exactly one trace file");
    let body = std::fs::read_to_string(entries[0].path()).expect("read trace");
    let lines: Vec<&str> = body.lines().collect();
    assert_eq!(lines.len(), 2, "expected 2 trace lines (one per step)");

    let v0: serde_json::Value = serde_json::from_str(lines[0]).expect("parse line 0");
    let v1: serde_json::Value = serde_json::from_str(lines[1]).expect("parse line 1");

    // Step 1 record.
    assert_eq!(v0["step_id"], "s1");
    assert_eq!(v0["agent_id"], "a");
    assert_eq!(v0["domain_id"], "test-pipeline");
    assert_eq!(
        v0["session_id"], v1["session_id"],
        "session must be stable across steps"
    );
    assert!(
        v0["duration_ms"].as_u64().is_some(),
        "duration_ms should be u64"
    );

    // Step 2 record: input must reference s1's output (template rendered).
    assert_eq!(v1["step_id"], "s2");
    assert_eq!(v1["agent_id"], "b");
    let s1_output = v0["output"].as_str().expect("s1 output is a string");
    let s2_input = v1["input"].to_string();
    assert!(
        s2_input.contains(s1_output),
        "s2 input should embed s1 output; s1={s1_output} s2={s2_input}"
    );
}
