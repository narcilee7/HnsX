//! CLI integration smoke tests.
//!
//! These tests spawn the compiled `hnsx` binary and exercise the subcommands
//! that do not require external credentials (validate, run/test with noop
//! adapter, local metrics/traces).

use std::io::Write;
use std::process::Command;

fn hnsx() -> Command {
    Command::new(env!("CARGO_BIN_EXE_hnsx"))
}

fn temp_yaml(prefix: &str, contents: &str) -> std::path::PathBuf {
    let mut dir = std::env::temp_dir();
    dir.push(format!("{}-{}.yaml", prefix, uuid::Uuid::new_v4()));
    let mut file = std::fs::File::create(&dir).expect("create temp yaml");
    file.write_all(contents.as_bytes())
        .expect("write temp yaml");
    dir
}

fn valid_domain() -> std::path::PathBuf {
    temp_yaml(
        "hnsx-valid",
        r#"
id: cli-smoke
version: 0.1.0
description: CLI smoke test domain.
agents:
  - id: echo
    description: echo agent
    model: { provider: openai, model: gpt-4o-mini }
    adapter: { timeout_seconds: 30 }
    prompt:
      template: "echo: {{input}}"
      variables: {}
workflow:
  entry: s1
  steps:
    - id: s1
      agent: echo
      output: out
"#,
    )
}

fn invalid_domain() -> std::path::PathBuf {
    temp_yaml(
        "hnsx-invalid",
        r#"
id: cli-smoke-bad
version: 0.1.0
description: invalid domain.
agents:
  - id: echo
    description: echo agent
    model: { provider: openai, model: gpt-4o-mini }
    adapter: {}
    prompt: { template: "t", variables: {} }
  - id: echo
    description: duplicate
    model: { provider: openai, model: gpt-4o-mini }
    adapter: {}
    prompt: { template: "t", variables: {} }
workflow:
  entry: s1
  steps:
    - id: s1
      agent: echo
"#,
    )
}

#[test]
fn validate_accepts_a_valid_domain() {
    let path = valid_domain();
    let output = hnsx()
        .args(["validate", "--domain", path.to_str().unwrap()])
        .output()
        .expect("spawn validate");

    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(
        output.status.success(),
        "validate should succeed for a valid domain. stderr: {}",
        String::from_utf8_lossy(&output.stderr)
    );
    assert!(stdout.contains("cli-smoke"), "stdout: {stdout}");
    assert!(stdout.contains("is valid"), "stdout: {stdout}");
}

#[test]
fn validate_rejects_an_invalid_domain() {
    let path = invalid_domain();
    let output = hnsx()
        .args(["validate", "--domain", path.to_str().unwrap()])
        .output()
        .expect("spawn validate");

    assert!(
        !output.status.success(),
        "validate should fail for an invalid domain"
    );
    let stderr = String::from_utf8_lossy(&output.stderr);
    assert!(
        stderr.contains("duplicate agent") || stderr.contains("invalid"),
        "stderr: {stderr}"
    );
}

#[test]
fn run_with_noop_adapter_executes_without_credentials() {
    let path = valid_domain();
    let output = hnsx()
        .args([
            "run",
            "--domain",
            path.to_str().unwrap(),
            "--adapter",
            "noop",
            "--trigger",
            r#"{"msg":"hi"}"#,
        ])
        .output()
        .expect("spawn run");

    assert!(
        output.status.success(),
        "run with noop adapter should succeed. stderr: {}",
        String::from_utf8_lossy(&output.stderr)
    );
}

#[test]
fn test_with_noop_adapter_executes_a_single_agent() {
    let path = valid_domain();
    let output = hnsx()
        .args([
            "test",
            "--domain",
            path.to_str().unwrap(),
            "--agent",
            "echo",
            "--adapter",
            "noop",
            "--input",
            r#"{"msg":"hi"}"#,
        ])
        .output()
        .expect("spawn test");

    assert!(
        output.status.success(),
        "test with noop adapter should succeed. stderr: {}",
        String::from_utf8_lossy(&output.stderr)
    );
}

#[test]
fn register_and_unregister_domain_with_control_plane() {
    let path = valid_domain();

    // Use an ephemeral SQLite database.
    let db_path = std::env::temp_dir().join(format!("hnsx-cp-{}.db", uuid::Uuid::new_v4()));

    // Start the control plane on a random port.
    let mut cp = hnsx()
        .args([
            "control-plane",
            "--addr",
            "127.0.0.1:0",
            "--db",
            db_path.to_str().unwrap(),
        ])
        .stdout(std::process::Stdio::piped())
        .spawn()
        .expect("spawn control-plane");

    let mut cp_stdout = cp.stdout.take().unwrap();
    let mut buf = [0u8; 256];
    let mut line = String::new();
    let addr: String = loop {
        let n = std::io::Read::read(&mut cp_stdout, &mut buf).expect("read cp stdout");
        if n == 0 {
            panic!("control-plane exited before printing address");
        }
        line.push_str(&String::from_utf8_lossy(&buf[..n]));
        if let Some(pos) = line.find("gRPC on ") {
            let rest = &line[pos + 8..];
            if let Some(end) = rest.find(" and") {
                break rest[..end].to_string();
            }
        }
    };

    let result = (|| {
        let register = hnsx()
            .args([
                "register",
                "--domain",
                path.to_str().unwrap(),
                "--control-plane",
                &format!("http://{addr}"),
            ])
            .output()
            .expect("spawn register");
        assert!(
            register.status.success(),
            "register should succeed. stderr: {}",
            String::from_utf8_lossy(&register.stderr)
        );
        let stdout = String::from_utf8_lossy(&register.stdout);
        assert!(
            stdout.contains("registered domain cli-smoke"),
            "stdout: {stdout}"
        );

        let unregister = hnsx()
            .args([
                "unregister",
                "--id",
                "cli-smoke",
                "--version",
                "0.1.0",
                "--control-plane",
                &format!("http://{addr}"),
            ])
            .output()
            .expect("spawn unregister");
        assert!(
            unregister.status.success(),
            "unregister should succeed. stderr: {}",
            String::from_utf8_lossy(&unregister.stderr)
        );
        Ok(()) as Result<(), Box<dyn std::any::Any + Send>>
    })();

    let _ = cp.kill();
    result.unwrap();
}

#[test]
fn metrics_and_traces_read_local_jsonl_files() {
    let trace_dir = std::env::temp_dir().join(format!("hnsx-traces-{}", uuid::Uuid::new_v4()));
    std::fs::create_dir_all(&trace_dir).expect("create trace dir");
    std::fs::write(
        trace_dir.join("s1.jsonl"),
        r#"{"session_id":"s1","domain_id":"d1","step_id":"step1","agent_id":"a","started_at_ms":1,"duration_ms":12,"input":{},"output":"hello"}
{"session_id":"s1","domain_id":"d1","step_id":"step2","agent_id":"a","started_at_ms":2,"duration_ms":20,"input":{},"output":"world"}
"#,
    )
    .expect("write trace file");

    let metrics = hnsx()
        .args([
            "metrics",
            "--trace-dir",
            trace_dir.to_str().unwrap(),
            "--domain-id",
            "d1",
        ])
        .output()
        .expect("spawn metrics");

    let stdout = String::from_utf8_lossy(&metrics.stdout);
    assert!(
        metrics.status.success(),
        "metrics should succeed. stderr: {}",
        String::from_utf8_lossy(&metrics.stderr)
    );
    assert!(stdout.contains("step_records: 2"), "stdout: {stdout}");
    assert!(stdout.contains("total_duration_ms: 32"), "stdout: {stdout}");

    let traces = hnsx()
        .args([
            "traces",
            "--trace-dir",
            trace_dir.to_str().unwrap(),
            "--domain-id",
            "d1",
        ])
        .output()
        .expect("spawn traces");

    let stdout = String::from_utf8_lossy(&traces.stdout);
    assert!(
        traces.status.success(),
        "traces should succeed. stderr: {}",
        String::from_utf8_lossy(&traces.stderr)
    );
    assert!(stdout.contains("session=s1"), "stdout: {stdout}");
    assert!(stdout.contains("step1"), "stdout: {stdout}");
    assert!(stdout.contains("step2"), "stdout: {stdout}");
}
