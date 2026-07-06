//! Sandbox trait and policy types. Sandbox is a first-class concept in HnsX,
//! not a side effect of an Adapter.

use std::collections::HashMap;

use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use crate::chunk::FileChange;
use crate::error::Result;

#[async_trait]
pub trait Sandbox: Send + Sync {
    async fn create(&self, spec: &SandboxSpec) -> Result<SandboxInstance>;
    async fn execute(&self, cmd: &str, env: &HashMap<String, String>) -> Result<ProcessHandle>;
    async fn read_file(&self, path: &str) -> Result<Vec<u8>>;
    async fn write_file(&self, path: &str, content: &[u8]) -> Result<()>;
    async fn list_changes(&self) -> Result<Vec<FileChange>>;
    async fn destroy(&self) -> Result<()>;
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SandboxInstance {
    pub id: String,
}

#[derive(Debug)]
pub struct ProcessHandle {
    _private: (),
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SandboxSpec {
    #[serde(default = "default_policy")]
    pub policy: SandboxPolicy,
}

fn default_policy() -> SandboxPolicy {
    SandboxPolicy::None
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum SandboxPolicy {
    /// No isolation; appropriate for pure network calls (OpenAI, Anthropic).
    None,
    /// Process-level isolation.
    Process,
    /// Linux namespace isolation. Used by Claude Code CLI.
    Namespace,
    /// Container-level isolation.
    Container,
    /// VM-level isolation.
    Vm,
}
