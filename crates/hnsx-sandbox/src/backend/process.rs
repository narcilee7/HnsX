//! Process-level hardening backend.
//!
//! This is the lowest-common-denominator backend that works on every supported
//! platform. It spawns commands via `tokio::process::Command` and scopes
//! filesystem operations to a temporary working directory. On Unix it also
//! applies rlimits where enabled.

use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::process::Stdio;
use std::sync::Arc;

use async_trait::async_trait;
use futures::StreamExt;
use hnsx_core::{
    chunk::FileChange,
    error::{Error, Result},
    sandbox::{LineStream, ProcessHandle, Sandbox, SandboxInstance, SandboxSpec},
};
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, ChildStderr, ChildStdout};
use tokio::sync::Mutex;

pub struct ProcessBackend;

impl ProcessBackend {
    pub fn new() -> Self {
        Self
    }
}

impl Default for ProcessBackend {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl Sandbox for ProcessBackend {
    async fn create(&self, _spec: &SandboxSpec) -> Result<SandboxInstance> {
        Ok(SandboxInstance {
            id: "process".to_string(),
        })
    }

    async fn execute(
        &self,
        cmd: &str,
        env: &HashMap<String, String>,
    ) -> Result<Box<dyn ProcessHandle>> {
        let work_dir = tempfile::tempdir()
            .map_err(|e| Error::Sandbox(format!("ProcessBackend workdir: {e}")))?;

        // Parse the command as a shell command. This keeps the API simple
        // (one string) while still supporting pipes and redirects when needed.
        // Security note: the command originates from the domain definition or
        // an adapter, not from arbitrary user input.
        let mut command = tokio::process::Command::new("sh");
        command.arg("-c").arg(cmd);
        command
            .current_dir(work_dir.path())
            .env_clear()
            .envs(env)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped());

        // Preserve PATH so that installed CLIs (node, python, etc.) remain
        // resolvable inside the sandbox. HOME/LANG are also safe to keep.
        if let Ok(path) = std::env::var("PATH") {
            command.env("PATH", path);
        }
        if let Ok(home) = std::env::var("HOME") {
            command.env("HOME", home);
        }
        if let Ok(lang) = std::env::var("LANG") {
            command.env("LANG", lang);
        }

        #[cfg(unix)]
        apply_unix_limits(&mut command);

        let child = command
            .spawn()
            .map_err(|e| Error::Sandbox(format!("ProcessBackend spawn '{cmd}': {e}")))?;

        Ok(Box::new(TokioProcessHandle::new(child, work_dir)))
    }

    async fn read_file(&self, _path: &str) -> Result<Vec<u8>> {
        // ProcessBackend does not track a persistent workdir across calls;
        // callers should read files relative to the command they executed.
        Err(Error::Unimplemented("ProcessBackend::read_file"))
    }

    async fn write_file(
        &self,
        _path: &str,
        _content: &[u8],
    ) -> Result<()> {
        Err(Error::Unimplemented("ProcessBackend::write_file"))
    }

    async fn list_changes(&self) -> Result<Vec<FileChange>> {
        Ok(Vec::new())
    }

    async fn destroy(&self) -> Result<()> {
        Ok(())
    }
}

#[cfg(unix)]
fn apply_unix_limits(command: &mut tokio::process::Command) {
    // Conservative defaults: 5 minutes CPU, 1 GiB address space.
    // These are soft limits only; the caller can tighten them via
    // `SandboxSpec` once it carries resource quotas.
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

/// A `ProcessHandle` backed by a `tokio::process::Child`.
pub struct TokioProcessHandle {
    child: Arc<Mutex<Child>>,
    stdout: Arc<Mutex<Option<ChildStdout>>>,
    stderr: Arc<Mutex<Option<ChildStderr>>>,
    /// Kept alive so the temporary directory is not deleted while the process
    /// may still hold open file descriptors inside it.
    _work_dir: tempfile::TempDir,
}

impl TokioProcessHandle {
    fn new(mut child: Child, work_dir: tempfile::TempDir) -> Self {
        let stdout = child.stdout.take();
        let stderr = child.stderr.take();
        Self {
            child: Arc::new(Mutex::new(child)),
            stdout: Arc::new(Mutex::new(stdout)),
            stderr: Arc::new(Mutex::new(stderr)),
            _work_dir: work_dir,
        }
    }

    fn line_stream(reader: Option<impl tokio::io::AsyncRead + Unpin + Send + 'static>) -> LineStream {
        let Some(reader) = reader else {
            return Box::pin(futures::stream::empty());
        };
        let lines = BufReader::new(reader).lines();
        Box::pin(futures::stream::unfold(lines, |mut lines| async move {
            match lines.next_line().await {
                Ok(Some(line)) => Some((Ok(line), lines)),
                Ok(None) => None,
                Err(e) => Some((Err(Error::Io(e)), lines)),
            }
        }))
    }
}

#[async_trait]
impl ProcessHandle for TokioProcessHandle {
    fn stdout(&self) -> LineStream {
        let stdout = self.stdout.clone();
        Box::pin(futures::stream::once(async move {
            let mut guard = stdout.lock().await;
            guard.take()
        })
        .flat_map(Self::line_stream))
    }

    fn stderr(&self) -> LineStream {
        let stderr = self.stderr.clone();
        Box::pin(futures::stream::once(async move {
            let mut guard = stderr.lock().await;
            guard.take()
        })
        .flat_map(Self::line_stream))
    }

    async fn kill(&self) -> Result<()> {
        let mut child = self.child.lock().await;
        child.kill().await.map_err(Error::Io)
    }

    async fn wait(&self) -> Result<std::process::ExitStatus> {
        let mut child = self.child.lock().await;
        child.wait().await.map_err(Error::Io)
    }
}

/// Resolve a relative path against a base directory and reject traversal
/// outside the base. Used by backends that do expose read/write.
pub fn resolve_sandbox_path(base: &Path, path: &str) -> Result<PathBuf> {
    let joined = base.join(path);
    let canonical = joined
        .canonicalize()
        .unwrap_or_else(|_| joined.clone());
    let base_canonical = base
        .canonicalize()
        .unwrap_or_else(|_| base.to_path_buf());
    if !canonical.starts_with(&base_canonical) {
        return Err(Error::Sandbox(format!(
            "path '{}' escapes sandbox root",
            path
        )));
    }
    Ok(joined)
}

#[cfg(test)]
mod tests {
    use super::*;
    use futures::StreamExt;

    #[tokio::test]
    async fn echo_command_streams_stdout() {
        let backend = ProcessBackend::new();
        let handle = backend
            .execute("echo hello world", &HashMap::new())
            .await
            .expect("execute");
        let mut stdout = handle.stdout();
        let line = stdout.next().await.expect("a line").expect("ok line");
        assert_eq!(line, "hello world");
        assert!(stdout.next().await.is_none());
    }

    #[tokio::test]
    async fn stderr_is_separate_stream() {
        let backend = ProcessBackend::new();
        let handle = backend
            .execute("echo err >&2", &HashMap::new())
            .await
            .expect("execute");
        let mut stderr = handle.stderr();
        let line = stderr.next().await.expect("a line").expect("ok line");
        assert_eq!(line, "err");
    }

    #[tokio::test]
    async fn env_is_passed_to_command() {
        let mut env = HashMap::new();
        env.insert("FOO".to_string(), "bar".to_string());
        let backend = ProcessBackend::new();
        let handle = backend
            .execute("echo $FOO", &env)
            .await
            .expect("execute");
        let mut stdout = handle.stdout();
        let line = stdout.next().await.expect("a line").expect("ok line");
        assert_eq!(line, "bar");
    }
}
