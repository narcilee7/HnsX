//! Sandbox trait and policy types. Sandbox is a first-class, cross-platform
//! concept in HnsX, not a side effect of an Adapter.
//!
//! The contract is: a `SandboxSpec` declares *how strong* the isolation should
//! be (`SandboxPolicy`) and optionally *which backend runtime* should provide
//! it (`SandboxRuntime`). The actual implementation is selected by the
//! platform-specific crate (`hnsx-sandbox`) or by the cloud deployment target.

use std::collections::HashMap;
use std::process::ExitStatus;

use async_trait::async_trait;
use futures::stream::BoxStream;
use serde::{Deserialize, Serialize};

use crate::chunk::FileChange;
use crate::error::Result;

/// A stream of lines read from a sandboxed process.
pub type LineStream = BoxStream<'static, Result<String>>;

#[async_trait]
pub trait Sandbox: Send + Sync {
    async fn create(&self, spec: &SandboxSpec) -> Result<SandboxInstance>;
    async fn execute(
        &self,
        instance: &SandboxInstance,
        cmd: &str,
        env: &HashMap<String, String>,
    ) -> Result<Box<dyn ProcessHandle>>;
    async fn read_file(&self, instance: &SandboxInstance, path: &str) -> Result<Vec<u8>>;
    async fn write_file(
        &self,
        instance: &SandboxInstance,
        path: &str,
        content: &[u8],
    ) -> Result<()>;
    async fn list_changes(&self, instance: &SandboxInstance) -> Result<Vec<FileChange>>;
    async fn destroy(&self, instance: &SandboxInstance) -> Result<()>;
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SandboxInstance {
    pub id: String,
}

/// Handle to a running sandboxed process. Backends return their own
/// implementation (e.g. a `tokio::process::Child` wrapper). Callers stream
/// stdout/stderr, can kill the process, and can wait for exit.
#[async_trait]
pub trait ProcessHandle: Send + Sync {
    /// Stream of lines from the process's stdout.
    fn stdout(&self) -> LineStream;
    /// Stream of lines from the process's stderr.
    fn stderr(&self) -> LineStream;
    /// Send SIGKILL (or platform equivalent) to the process.
    async fn kill(&self) -> Result<()>;
    /// Wait for the process to exit and return its status.
    async fn wait(&self) -> Result<ExitStatus>;
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct SandboxSpec {
    #[serde(default)]
    pub policy: SandboxPolicy,
    /// Optional backend runtime hint. `Auto` lets the platform pick the best
    /// available implementation for the requested policy.
    #[serde(default)]
    pub runtime: SandboxRuntime,
    /// Maximum wall-clock time for a single `execute` call (seconds).
    #[serde(default)]
    pub timeout_seconds: Option<u64>,
    /// If true, `write_file` and command writes are rejected. Backends that
    /// cannot enforce this should document it as best-effort.
    #[serde(default)]
    pub read_only: bool,
    /// Additional paths (relative to the sandbox root or absolute) that the
    /// command is allowed to read/write. Currently advisory.
    #[serde(default)]
    pub allowed_paths: Vec<String>,
    /// If true, the backend should attempt to disable network access. This is
    /// only enforceable on some platforms (Linux namespaces, containers).
    #[serde(default)]
    pub disable_network: bool,
    /// Soft memory limit in MiB. Enforced via rlimits on Unix; advisory elsewhere.
    #[serde(default)]
    pub max_memory_mb: Option<u64>,
    /// Soft CPU time limit in seconds. Enforced via rlimits on Unix; advisory elsewhere.
    #[serde(default)]
    pub max_cpu_seconds: Option<u64>,
}

/// Isolation strength. This is platform-neutral: the same `SandboxPolicy`
/// produces equivalent *security outcomes* on Linux, macOS, Windows, or in the
/// cloud, even though the underlying technology differs.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq, Default)]
#[serde(rename_all = "lowercase")]
pub enum SandboxPolicy {
    /// No isolation; appropriate for pure network calls (OpenAI, Anthropic).
    #[default]
    None,
    /// Process-level isolation (least-privilege process tokens, seatbelt,
    /// job objects, rlimits, etc.).
    Process,
    /// OS-level namespace / seatbelt / job-object isolation.
    Namespace,
    /// Container-level isolation (Docker, containerd, podman, etc.).
    Container,
    /// VM-level isolation (Firecracker, Kata, Cloud Hypervisor, etc.).
    Vm,
}

/// Concrete backend runtime that provides the isolation. `Auto` is the default
/// and maps a `SandboxPolicy` to the best local or cloud backend.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq, Default)]
#[serde(rename_all = "kebab-case")]
pub enum SandboxRuntime {
    /// Pick the best available backend for the current platform / deployment.
    #[default]
    Auto,
    /// No backend; equivalent to `SandboxPolicy::None`.
    None,
    /// Process-level hardening available on every OS.
    Process,
    /// Linux namespaces + landlock + seccomp + cgroups.
    LinuxNamespace,
    /// macOS seatbelt / sandbox profile.
    MacosSeatbelt,
    /// Windows job object + ACL / sandbox.
    WindowsJobObject,
    /// OCI container runtime (Docker, containerd, podman, ...).
    Container,
    /// Micro-VM (Firecracker, Kata, Cloud Hypervisor, ...).
    MicroVm,
}
