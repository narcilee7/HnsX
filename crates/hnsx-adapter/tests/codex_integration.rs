//! Hermetic integration test for the Codex CLI adapter.
//!
//! The real `codex` CLI may not be installed in CI, so we create a mock
//! `codex` shell script, prepend its directory to PATH, and verify that
//! `CodexAgent` spawns it inside the process sandbox and streams stdout.

use std::collections::HashMap;
use std::io::Write;
use std::sync::Arc;

use futures::StreamExt;
use serde_json::json;

use hnsx_adapter::CodexAgent;
use hnsx_core::agent::{Agent, AgentSpec, InvokeContext, ModelRef, PromptTemplate, Provider};
use hnsx_core::chunk::Chunk;
use hnsx_sandbox::backend::process::ProcessBackend;

fn codex_spec(command: &str) -> AgentSpec {
    AgentSpec {
        id: "coder".into(),
        description: "x".into(),
        model: ModelRef {
            provider: Provider::Codex,
            model: "gpt-4o".into(),
            endpoint: None,
        },
        adapter: hnsx_core::agent::AdapterConfig {
            timeout_seconds: None,
            extra: json!({"command": command}),
        },
        tools: vec![],
        prompt: PromptTemplate {
            template: "You are a coder.".into(),
            variables: json!({}),
        },
        sandbox: None,
        memory_window: None,
    }
}

fn mock_codex_path() -> (tempfile::TempDir, std::path::PathBuf) {
    let dir = tempfile::tempdir().expect("tempdir");
    let script_path = dir.path().join("codex");
    #[cfg(unix)]
    {
        let mut f = std::fs::File::create(&script_path).expect("create script");
        f.write_all(b"#!/bin/sh\necho 'Codex says hi'\n").expect("write");
        use std::os::unix::fs::PermissionsExt;
        let mut perms = std::fs::metadata(&script_path).unwrap().permissions();
        perms.set_mode(0o755);
        std::fs::set_permissions(&script_path, perms).expect("chmod");
    }
    #[cfg(not(unix))]
    {
        // Windows: use a .cmd wrapper.
        let mut f = std::fs::File::create(&script_path.with_extension("cmd")).expect("create script");
        f.write_all(b"@echo off\necho Codex says hi\n").expect("write");
    }
    (dir, script_path)
}

#[tokio::test(flavor = "multi_thread")]
async fn codex_agent_spawns_cli_and_streams_stdout() {
    let (_dir, script_path) = mock_codex_path();
    let script_dir = script_path.parent().unwrap().to_path_buf();

    // Prepend the mock script directory to PATH so the bare `codex` name
    // resolves to our mock. We also use the absolute path in the agent config
    // to guarantee the mock is invoked even if PATH ordering is changed by the
    // sandbox backend.
    let mut env: HashMap<String, String> = HashMap::new();
    let new_path = if let Ok(existing) = std::env::var("PATH") {
        format!("{}:{existing}", script_dir.display())
    } else {
        script_dir.to_string_lossy().to_string()
    };
    env.insert("PATH".into(), new_path);

    let sandbox: Arc<dyn hnsx_core::Sandbox + Send + Sync + 'static> =
        Arc::new(ProcessBackend::new());
    let agent = CodexAgent::new(sandbox, &codex_spec(&script_path.to_string_lossy()));

    let mut stream = agent
        .invoke(
            json!({"task": "hello"}),
            InvokeContext {
                session_id: "s1".into(),
                domain_id: "d1".into(),
                agent_id: "coder".into(),
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
    assert_eq!(texts.join(""), "Codex says hi");
}
