//! Codex CLI adapter.
//!
//! Spawns the local `codex` CLI (GitHub Copilot's command-line agent) inside
//! a sandbox, streams its stdout/stderr back as `Chunk`s, and reports
//! filesystem changes as artifacts.
//!
//! The CLI invocation is intentionally simple: `codex <flags> <prompt>`.
//! Non-interactive flags can be supplied via `AgentSpec.adapter.extra.flags`
//! (a string or array of strings). If no flags are configured, the adapter
//! falls back to `--no-confirm` as a sensible default for automated runs.

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use async_stream::stream;
use async_trait::async_trait;
use futures::stream::{BoxStream, StreamExt};
use serde_json::Value;

use hnsx_core::agent::{Agent, AgentSchema, AgentSpec, HealthStatus, InvokeContext};
use hnsx_core::chunk::Chunk;
use hnsx_core::error::{Error, Result};
use hnsx_core::sandbox::{Sandbox, SandboxPolicy, SandboxRuntime, SandboxSpec};

/// Agent that shells out to the Codex CLI.
pub struct CodexAgent {
    sandbox: Arc<dyn Sandbox + Send + Sync + 'static>,
    system_prompt: String,
    sandbox_spec: SandboxSpec,
    flags: Vec<String>,
    command: String,
}

impl CodexAgent {
    pub fn new(sandbox: Arc<dyn Sandbox + Send + Sync + 'static>, spec: &AgentSpec) -> Self {
        let sandbox_spec = spec.sandbox.clone().unwrap_or(SandboxSpec {
            policy: SandboxPolicy::Process,
            runtime: SandboxRuntime::Auto,
        });
        let flags = extract_flags(&spec.adapter.extra);
        let command = spec
            .adapter
            .extra
            .get("command")
            .and_then(Value::as_str)
            .unwrap_or("codex")
            .to_string();
        Self {
            sandbox,
            system_prompt: spec.prompt.template.clone(),
            sandbox_spec,
            flags,
            command,
        }
    }
}

fn extract_flags(extra: &Value) -> Vec<String> {
    if let Some(s) = extra.get("flags").and_then(Value::as_str) {
        return s.split_whitespace().map(String::from).collect();
    }
    if let Some(arr) = extra.get("flags").and_then(Value::as_array) {
        return arr
            .iter()
            .filter_map(|v| v.as_str().map(String::from))
            .collect();
    }
    // Sensible default for automated / sandboxed invocation: skip human
    // confirmation prompts.
    vec!["--no-confirm".into()]
}

fn shell_escape(s: &str) -> String {
    format!("'{}'", s.replace('\'', "'\\''"))
}

#[async_trait]
impl Agent for CodexAgent {
    async fn invoke(
        &self,
        input: Value,
        _ctx: InvokeContext,
    ) -> Result<BoxStream<'static, Chunk>> {
        let sandbox = self.sandbox.clone();
        let sandbox_spec = self.sandbox_spec.clone();
        let system_prompt = self.system_prompt.clone();
        let flags = self.flags.clone();
        let command = self.command.clone();

        let _instance = sandbox.create(&sandbox_spec).await?;

        let prompt = format!("{}\n\nTask input: {}", system_prompt, input);
        let flag_str = flags.join(" ");
        let cmd = format!("{command} {flag_str} {}", shell_escape(&prompt));

        let handle = sandbox
            .execute(&cmd, &HashMap::new())
            .await
            .map_err(|e| Error::Adapter(format!("codex execute: {e}")))?;

        Ok(Box::pin(stream! {
            let mut stdout = handle.stdout();
            let mut stderr = handle.stderr();
            let idle_timeout = Duration::from_secs(30);

            loop {
                tokio::select! {
                    line = tokio::time::timeout(idle_timeout, stdout.next()) => {
                        match line {
                            Ok(Some(Ok(l))) => yield Chunk::text(l),
                            Ok(Some(Err(e))) => {
                                yield Chunk::error(format!("stdout error: {e}"));
                                return;
                            }
                            Ok(None) => break,
                            Err(_) => {
                                yield Chunk::error("Idle timeout; killing Codex CLI");
                                let _ = handle.kill().await;
                                return;
                            }
                        }
                    }
                    line = tokio::time::timeout(idle_timeout, stderr.next()) => {
                        match line {
                            Ok(Some(Ok(l))) => yield Chunk::error(l),
                            Ok(Some(Err(e))) => {
                                yield Chunk::error(format!("stderr error: {e}"));
                                return;
                            }
                            Ok(None) => {}
                            Err(_) => {
                                yield Chunk::error("Idle timeout; killing Codex CLI");
                                let _ = handle.kill().await;
                                return;
                            }
                        }
                    }
                }
            }

            let _ = handle.wait().await;

            match sandbox.list_changes().await {
                Ok(changes) if !changes.is_empty() => {
                    yield Chunk::artifact(hnsx_core::chunk::Artifact::FileChanges(changes));
                }
                Ok(_) => {}
                Err(e) => yield Chunk::error(format!("list_changes failed: {e}")),
            }
        }))
    }

    async fn health(&self) -> HealthStatus {
        let probe_spec = SandboxSpec {
            policy: SandboxPolicy::None,
            runtime: SandboxRuntime::None,
        };
        match self.sandbox.create(&probe_spec).await {
            Ok(_) => match self.sandbox.execute("codex --version", &HashMap::new()).await {
                Ok(_) => HealthStatus {
                    healthy: true,
                    message: Some("codex CLI found".to_string()),
                },
                Err(e) => HealthStatus {
                    healthy: false,
                    message: Some(format!("codex CLI not available: {e}")),
                },
            },
            Err(e) => HealthStatus {
                healthy: false,
                message: Some(format!("sandbox create failed: {e}")),
            },
        }
    }

    async fn schema(&self) -> AgentSchema {
        AgentSchema {
            name: "codex".to_string(),
            input_schema: serde_json::json!({"type": "object"}),
            output_schema: serde_json::json!({"type": "string"}),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use hnsx_core::agent::{AdapterConfig, ModelRef, PromptTemplate, Provider};
    use serde_json::json;

    fn dummy_spec() -> AgentSpec {
        AgentSpec {
            id: "coder".into(),
            description: "coder".into(),
            model: ModelRef {
                provider: Provider::Codex,
                model: "gpt-4o".into(),
                endpoint: None,
            },
            adapter: AdapterConfig {
                timeout_seconds: None,
                extra: json!({}),
            },
            tools: vec![],
            prompt: PromptTemplate {
                template: "You are a coder.".into(),
                variables: json!({}),
            },
            sandbox: None,
        }
    }

    #[test]
    fn default_flags_use_no_confirm() {
        let spec = dummy_spec();
        // Since we can't easily build a sandbox here, just test flag parsing.
        assert_eq!(extract_flags(&spec.adapter.extra), vec!["--no-confirm"]);
    }

    #[test]
    fn extracts_string_flags() {
        let extra = json!({"flags": "-q --no-confirm"});
        assert_eq!(extract_flags(&extra), vec!["-q", "--no-confirm"]);
    }

    #[test]
    fn extracts_array_flags() {
        let extra = json!({"flags": ["--approval-policy", "never-confirm"]});
        assert_eq!(
            extract_flags(&extra),
            vec!["--approval-policy", "never-confirm"]
        );
    }

    #[test]
    fn shell_escape_quotes() {
        assert_eq!(shell_escape("hello"), "'hello'");
        assert_eq!(shell_escape("it's"), "'it'\\''s'");
    }
}
