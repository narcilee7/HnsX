//! Python tool: run a Python script inside a sandboxed process.
//!
//! Phase 3.5 provides a minimal, safe wrapper:
//! - The script can be inline (`script` config) or a file path (`entrypoint`).
//! - Per-call `args` are passed as JSON on stdin.
//! - Execution is via `python3`; timeout and rlimits are applied through the
//!   same `tokio::process` + `pre_exec` rlimit hooks used by `ProcessBackend`.

use std::process::Stdio;
use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use serde::Deserialize;
use serde_json::{json, Value};

use hnsx_core::agent::ToolKind;
use hnsx_core::error::{Error, Result};
use hnsx_core::tool::Tool;

const DEFAULT_TIMEOUT_MS: u64 = 30_000;

#[derive(Debug, Clone, Deserialize)]
pub struct PythonConfig {
    /// Inline Python source code. Mutually exclusive with `entrypoint`.
    #[serde(default)]
    pub script: Option<String>,
    /// Path to a Python file inside the agent's working directory.
    #[serde(default)]
    pub entrypoint: Option<String>,
    /// Timeout in ms. Defaults to 30s.
    #[serde(default)]
    pub timeout_ms: Option<u64>,
}

#[derive(Debug)]
pub struct PythonTool {
    name: String,
    config: Value,
    invocation: PythonInvocation,
    timeout: Duration,
}

#[derive(Debug, Clone)]
enum PythonInvocation {
    Inline(String),
    Entrypoint(String),
}

impl PythonTool {
    pub fn new(name: impl Into<String>, config: Value) -> Result<Arc<Self>> {
        let cfg: PythonConfig = serde_json::from_value(config.clone())
            .map_err(|e| Error::Adapter(format!("PythonTool config: {e}")))?;

        let invocation = match (cfg.script, cfg.entrypoint) {
            (Some(script), None) => PythonInvocation::Inline(script),
            (None, Some(entrypoint)) => PythonInvocation::Entrypoint(entrypoint),
            (Some(_), Some(_)) => {
                return Err(Error::Adapter(
                    "PythonTool: specify either `script` or `entrypoint`, not both".into(),
                ));
            }
            (None, None) => {
                return Err(Error::Adapter(
                    "PythonTool: either `script` or `entrypoint` is required".into(),
                ));
            }
        };

        let timeout = Duration::from_millis(cfg.timeout_ms.unwrap_or(DEFAULT_TIMEOUT_MS));

        Ok(Arc::new(Self {
            name: name.into(),
            config,
            invocation,
            timeout,
        }))
    }
}

#[async_trait]
impl Tool for PythonTool {
    fn name(&self) -> &str {
        &self.name
    }

    fn kind(&self) -> ToolKind {
        ToolKind::Python
    }

    fn config(&self) -> &Value {
        &self.config
    }

    async fn invoke(&self, args: Value) -> Result<Value> {
        let args_json = serde_json::to_string(&args)
            .map_err(|e| Error::Adapter(format!("PythonTool serialize args: {e}")))?;

        let mut command = tokio::process::Command::new("python3");
        match &self.invocation {
            PythonInvocation::Inline(script) => {
                command.arg("-c").arg(script);
            }
            PythonInvocation::Entrypoint(entrypoint) => {
                command.arg(entrypoint);
            }
        }

        command
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .kill_on_drop(true);

        #[cfg(unix)]
        apply_unix_limits(&mut command);

        let mut child = command
            .spawn()
            .map_err(|e| Error::Adapter(format!("PythonTool spawn: {e}")))?;

        // Write args to stdin and close it.
        if let Some(stdin) = child.stdin.take() {
            let mut stdin = stdin;
            tokio::io::AsyncWriteExt::write_all(&mut stdin,
                args_json.as_bytes(),
            )
            .await
            .map_err(|e| Error::Adapter(format!("PythonTool write stdin: {e}")))?;
            // drop stdin to close the pipe
        }

        let result = tokio::time::timeout(self.timeout, child.wait_with_output()).await;
        let output: std::process::Output = result
            .map_err(|_| Error::Adapter("PythonTool timed out".into()))?
            .map_err(|e| Error::Adapter(format!("PythonTool output: {e}")))?;

        let stdout = String::from_utf8_lossy(&output.stdout).to_string();
        let stderr = String::from_utf8_lossy(&output.stderr).to_string();

        // Try to parse stdout as JSON; fall back to string.
        let result: Value = serde_json::from_str(&stdout).unwrap_or_else(|_| Value::String(stdout.clone()));

        Ok(json!({
            "ok": output.status.success(),
            "exit_code": output.status.code(),
            "result": result,
            "stdout": stdout,
            "stderr": stderr,
        }))
    }
}

#[cfg(unix)]
fn apply_unix_limits(command: &mut tokio::process::Command) {
    unsafe {
        command.pre_exec(|| {
            let _ = nix::sys::resource::setrlimit(
                nix::sys::resource::Resource::RLIMIT_CPU,
                300,
                600,
            );
            let _ = nix::sys::resource::setrlimit(
                nix::sys::resource::Resource::RLIMIT_AS,
                1024 * 1024 * 1024,
                2 * 1024 * 1024 * 1024,
            );
            Ok(())
        });
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn inline_script_reads_args_from_stdin() {
        let tool = PythonTool::new(
            "calc",
            json!({
                "script": "import sys, json; args = json.load(sys.stdin); print(json.dumps({'sum': args['a'] + args['b']}))"
            }),
        )
        .expect("build");

        let out = tool.invoke(json!({"a": 2, "b": 3})).await.expect("invoke");
        assert_eq!(out["ok"], true);
        assert_eq!(out["result"]["sum"], 5);
    }

    #[tokio::test]
    async fn timeout_kills_long_running_script() {
        let tool = PythonTool::new(
            "slow",
            json!({
                "script": "import time; time.sleep(10)",
                "timeout_ms": 100
            }),
        )
        .expect("build");

        let err = tool.invoke(json!({})).await.unwrap_err();
        assert!(format!("{err}").contains("timed out"), "got: {err:?}");
    }

    #[tokio::test]
    async fn entrypoint_invokes_file() {
        let dir = tempfile::tempdir().expect("tempdir");
        let script = dir.path().join("echo.py");
        tokio::fs::write(
            &script,
            "import sys, json; args = json.load(sys.stdin); print(json.dumps({'echo': args['msg']}))",
        )
        .await
        .expect("write script");

        let tool = PythonTool::new(
            "echo",
            json!({"entrypoint": script.to_string_lossy().to_string()}),
        )
        .expect("build");

        let out = tool.invoke(json!({"msg": "hi"})).await.expect("invoke");
        assert_eq!(out["ok"], true);
        assert_eq!(out["result"]["echo"], "hi");
    }

    #[test]
    fn rejects_both_script_and_entrypoint() {
        let err = PythonTool::new(
            "x",
            json!({"script": "x", "entrypoint": "y.py"}),
        )
        .unwrap_err();
        assert!(
            matches!(err, Error::Adapter(ref m) if m.contains("not both")),
            "got: {err:?}"
        );
    }
}
