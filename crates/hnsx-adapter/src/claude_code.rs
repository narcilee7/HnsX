//! Claude Code CLI adapter.
//!
//! Spawns the local `claude` CLI inside a sandbox, streams its stdout/stderr
//! back as `Chunk`s, and reports filesystem changes as artifacts.

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use async_stream::stream;
use async_trait::async_trait;
use futures::stream::{BoxStream, StreamExt};
use serde_json::Value;

use hnsx_core::adapter::{Adapter, RuntimeContext};
use hnsx_core::agent::{AgentSpec, HealthStatus};
use hnsx_core::chunk::Chunk;
use hnsx_core::error::{Error, Result};
use hnsx_core::sandbox::{Sandbox, SandboxPolicy, SandboxRuntime, SandboxSpec};
use hnsx_core::tool::ToolRegistry;

/// Adapter that shells out to the Claude Code CLI.
pub struct ClaudeCodeAdapter {
    sandbox: Arc<dyn Sandbox + Send + Sync + 'static>,
    system_prompt: String,
    sandbox_spec: SandboxSpec,
}

impl ClaudeCodeAdapter {
    pub fn new(sandbox: Arc<dyn Sandbox>, spec: &AgentSpec) -> Self {
        let sandbox_spec = spec.sandbox.clone().unwrap_or(SandboxSpec {
            policy: SandboxPolicy::Namespace,
            runtime: SandboxRuntime::Auto,
        });
        Self {
            sandbox,
            system_prompt: spec.prompt.template.clone(),
            sandbox_spec,
        }
    }

    /// No-op for interface consistency: the Claude Code CLI handles its own
    /// tool use internally.
    pub fn with_tools(self, _tools: ToolRegistry) -> Self {
        self
    }
}

fn shell_escape(s: &str) -> String {
    format!("'{}'", s.replace('\'', "'\\''"))
}

#[async_trait]
impl Adapter for ClaudeCodeAdapter {
    async fn prepare(&self, _config: &Value) -> Result<RuntimeContext> {
        Ok(RuntimeContext {
            session_id: uuid::Uuid::new_v4().to_string(),
            config: Value::Null,
        })
    }

    async fn invoke(&self, input: &Value, _ctx: &RuntimeContext) -> Result<BoxStream<'static, Chunk>> {
        let sandbox = self.sandbox.clone();
        let sandbox_spec = self.sandbox_spec.clone();
        let system_prompt = self.system_prompt.clone();

        let _instance = sandbox.create(&sandbox_spec).await?;

        // Build the prompt from the configured system prompt plus the task
        // input. The system prompt carries the agent's role; the input carries
        // the per-step payload (e.g. a diff).
        let prompt = format!("{}\n\nTask input: {}", system_prompt, input);

        // Use `claude -p` for non-interactive, single-shot output.
        // `--bare` disables LSP, plugin sync, keychain reads, and CLAUDE.md
        // discovery, which is what we want inside a sandbox.
        // `--allow-dangerously-skip-permissions` lets the CLI run without
        // prompting for human approval inside each tool call. This is required
        // for any automated / sandboxed invocation.
        let cmd = format!(
            "claude -p --bare --allow-dangerously-skip-permissions {}",
            shell_escape(&prompt)
        );

        let handle = sandbox
            .execute(&cmd, &HashMap::new())
            .await
            .map_err(|e| Error::Adapter(format!("claude-code execute: {e}")))?;

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
                                yield Chunk::error("Idle timeout; killing Claude Code CLI");
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
                                yield Chunk::error("Idle timeout; killing Claude Code CLI");
                                let _ = handle.kill().await;
                                return;
                            }
                        }
                    }
                }
            }

            // Wait for graceful exit so list_changes is meaningful.
            let exit_status = match handle.wait().await {
                Ok(s) => s,
                Err(e) => {
                    yield Chunk::error(format!("claude-code wait failed: {e}"));
                    return;
                }
            };

            if !exit_status.success() {
                let code = exit_status.code().map(|c| c.to_string()).unwrap_or_else(|| "signal".into());
                yield Chunk::error(format!("claude-code exited with code {code}"));
                return;
            }

            match sandbox.list_changes().await {
                Ok(changes) if !changes.is_empty() => {
                    yield Chunk::artifact(hnsx_core::chunk::Artifact::FileChanges(changes));
                }
                Ok(_) => {}
                Err(e) => yield Chunk::error(format!("list_changes failed: {e}")),
            }
        }))
    }

    async fn teardown(&self, _ctx: &RuntimeContext) -> Result<()> {
        Ok(())
    }

    async fn health(&self) -> HealthStatus {
        // A lightweight probe: run `claude --version` inside a none sandbox.
        let probe_spec = SandboxSpec {
            policy: SandboxPolicy::None,
            runtime: SandboxRuntime::None,
        };
        match self.sandbox.create(&probe_spec).await {
            Ok(_) => match self.sandbox.execute("claude --version", &HashMap::new()).await {
                Ok(_) => HealthStatus {
                    healthy: true,
                    message: Some("claude CLI found".to_string()),
                },
                Err(e) => HealthStatus {
                    healthy: false,
                    message: Some(format!("claude CLI not available: {e}")),
                },
            },
            Err(e) => HealthStatus {
                healthy: false,
                message: Some(format!("sandbox create failed: {e}")),
            },
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
            id: "reviewer".into(),
            description: "reviewer".into(),
            model: ModelRef {
                provider: Provider::ClaudeCode,
                model: "sonnet".into(),
                endpoint: None,
            },
            adapter: AdapterConfig {
                timeout_seconds: None,
                extra: json!({}),
            },
            tools: vec![],
            prompt: PromptTemplate {
                template: "You are a code reviewer.".into(),
                variables: json!({}),
            },
            sandbox: None,
            memory_window: None,
        }
    }

    #[test]
    fn shell_escape_quotes() {
        assert_eq!(shell_escape("hello"), "'hello'");
        assert_eq!(shell_escape("it's"), "'it'\\''s'");
    }
}
