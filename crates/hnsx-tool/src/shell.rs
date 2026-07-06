//! Shell tool: whitelisted command execution.
//!
//! For 3.3 the tool runs commands in the host process via `std::process::Command`.
//! The Phase 2 sandbox will move execution into a namespace so a leaked
//! prompt can't damage the host. Until then, only commands in the
//! configured whitelist are allowed.

use std::process::Stdio;
use std::sync::Arc;

use async_trait::async_trait;
use serde::Deserialize;
use serde_json::{json, Value};

use hnsx_core::agent::ToolKind;
use hnsx_core::error::{Error, Result};
use hnsx_core::tool::Tool;

#[derive(Debug, Clone, Deserialize)]
pub struct ShellConfig {
    /// Whitelisted command basenames (e.g. `["cat", "grep", "git"]`).
    pub allow: Vec<String>,
    /// Working directory. Defaults to current.
    #[serde(default)]
    pub cwd: Option<String>,
}

pub struct ShellTool {
    name: String,
    config: Value,
    allow: Vec<String>,
    cwd: Option<String>,
}

impl ShellTool {
    pub fn new(name: impl Into<String>, config: Value) -> Result<Arc<Self>> {
        let cfg: ShellConfig = serde_json::from_value(config.clone())
            .map_err(|e| Error::Adapter(format!("ShellTool config: {e}")))?;
        if cfg.allow.is_empty() {
            return Err(Error::Adapter(
                "ShellTool config: `allow` must list at least one command".into(),
            ));
        }
        Ok(Arc::new(Self {
            name: name.into(),
            config,
            allow: cfg.allow,
            cwd: cfg.cwd,
        }))
    }
}

#[async_trait]
impl Tool for ShellTool {
    fn name(&self) -> &str {
        &self.name
    }
    fn kind(&self) -> ToolKind {
        ToolKind::Shell
    }
    fn config(&self) -> &Value {
        &self.config
    }

    async fn invoke(&self, args: Value) -> Result<Value> {
        let cmd = args
            .get("cmd")
            .and_then(Value::as_str)
            .ok_or_else(|| Error::Adapter("ShellTool: `cmd` is required".into()))?;
        let cmd_args: Vec<String> = args
            .get("args")
            .and_then(Value::as_array)
            .map(|a| {
                a.iter()
                    .filter_map(|v| v.as_str().map(String::from))
                    .collect()
            })
            .unwrap_or_default();

        if !self.allow.iter().any(|a| a == cmd) {
            return Err(Error::Adapter(format!(
                "ShellTool: `{cmd}` is not in the whitelist (allowed: {:?})",
                self.allow
            )));
        }

        let mut command = std::process::Command::new(cmd);
        command.args(&cmd_args);
        if let Some(cwd) = &self.cwd {
            command.current_dir(cwd);
        }
        command.stdin(Stdio::null()).stdout(Stdio::piped()).stderr(Stdio::piped());

        let output = command
            .output()
            .map_err(|e| Error::Adapter(format!("ShellTool spawn `{cmd}`: {e}")))?;

        let stdout = String::from_utf8_lossy(&output.stdout).to_string();
        let stderr = String::from_utf8_lossy(&output.stderr).to_string();
        let exit_code = output.status.code().unwrap_or(-1);

        Ok(json!({
            "cmd": cmd,
            "args": cmd_args,
            "exit_code": exit_code,
            "ok": output.status.success(),
            "stdout": stdout,
            "stderr": stderr,
        }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn allow(c: &[&str]) -> Value {
        json!({"allow": c})
    }

    #[test]
    fn rejects_empty_allow() {
        let err = match ShellTool::new("s", json!({"allow": []})) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(matches!(err, Error::Adapter(ref m) if m.contains("allow")), "got: {err:?}");
    }

    #[test]
    fn rejects_unknown_command() {
        let tool = ShellTool::new("s", allow(&["cat"])).expect("build");
        let err = match futures::executor::block_on(tool.invoke(json!({"cmd": "rm"}))) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(matches!(err, Error::Adapter(ref m) if m.contains("whitelist")), "got: {err:?}");
    }

    #[test]
    fn runs_whitelisted_cat() {
        let tool = ShellTool::new("s", allow(&["cat"])).expect("build");
        let out = futures::executor::block_on(tool.invoke(json!({
            "cmd": "cat",
            "args": []
        })))
        .expect("invoke");
        // No stdin -> cat exits 0 with empty stdout.
        assert_eq!(out["ok"], true);
        assert_eq!(out["exit_code"], 0);
    }

    #[test]
    fn missing_cmd_arg_errors() {
        let tool = ShellTool::new("s", allow(&["cat"])).expect("build");
        let err = match futures::executor::block_on(tool.invoke(json!({}))) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(matches!(err, Error::Adapter(ref m) if m.contains("`cmd`")), "got: {err:?}");
    }
}
