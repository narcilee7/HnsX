//! Process-level hardening backend.
//!
//! This is the lowest-common-denominator backend that works on every supported
//! platform. It spawns commands via `tokio::process::Command` inside a persistent
//! per-instance working directory. On Unix it applies rlimits where configured.

use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::process::Stdio;
use std::sync::{Arc, Mutex};

use async_trait::async_trait;
use futures::StreamExt;
use hnsx_core::{
    chunk::{FileChange, FileChangeKind},
    error::{Error, Result},
    sandbox::{LineStream, ProcessHandle, Sandbox, SandboxInstance, SandboxSpec},
};
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, ChildStderr, ChildStdout};
use tokio::sync::Mutex as TokioMutex;

pub struct ProcessBackend {
    instances: Mutex<HashMap<String, Arc<InstanceState>>>,
}

struct InstanceState {
    dir: Arc<tempfile::TempDir>,
    spec: SandboxSpec,
}

impl ProcessBackend {
    pub fn new() -> Self {
        Self {
            instances: Mutex::new(HashMap::new()),
        }
    }

    fn get_state(&self, id: &str) -> Result<Arc<InstanceState>> {
        let guard = self
            .instances
            .lock()
            .map_err(|e| Error::Sandbox(format!("ProcessBackend instance lock poisoned: {e}")))?;
        guard
            .get(id)
            .cloned()
            .ok_or_else(|| Error::Sandbox(format!("sandbox instance {id} not found")))
    }
}

impl Default for ProcessBackend {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl Sandbox for ProcessBackend {
    async fn create(&self, spec: &SandboxSpec) -> Result<SandboxInstance> {
        let id = uuid::Uuid::new_v4().to_string();
        let dir = Arc::new(
            tempfile::tempdir()
                .map_err(|e| Error::Sandbox(format!("ProcessBackend workdir: {e}")))?,
        );

        {
            let mut guard = self.instances.lock().map_err(|e| {
                Error::Sandbox(format!("ProcessBackend instance lock poisoned: {e}"))
            })?;
            guard.insert(
                id.clone(),
                Arc::new(InstanceState {
                    dir,
                    spec: spec.clone(),
                }),
            );
        }

        Ok(SandboxInstance { id })
    }

    async fn execute(
        &self,
        instance: &SandboxInstance,
        cmd: &str,
        env: &HashMap<String, String>,
    ) -> Result<Box<dyn ProcessHandle>> {
        let state = self.get_state(&instance.id)?;
        let work_dir = state.dir.path().to_path_buf();
        let spec = state.spec.clone();

        // Parse the command as a shell command. This keeps the API simple
        // (one string) while still supporting pipes and redirects when needed.
        let mut command = tokio::process::Command::new("sh");
        command.arg("-c").arg(cmd);
        command
            .current_dir(&work_dir)
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
        apply_unix_limits(&mut command, &spec);

        let child = command
            .spawn()
            .map_err(|e| Error::Sandbox(format!("ProcessBackend spawn '{cmd}': {e}")))?;

        Ok(Box::new(TokioProcessHandle::new(
            child,
            state,
            spec.timeout_seconds,
        )))
    }

    async fn read_file(&self, instance: &SandboxInstance, path: &str) -> Result<Vec<u8>> {
        let state = self.get_state(&instance.id)?;
        let resolved = resolve_sandbox_path(state.dir.path(), path)?;
        tokio::fs::read(&resolved)
            .await
            .map_err(|e| Error::Sandbox(format!("read_file '{}': {e}", path)))
    }

    async fn write_file(
        &self,
        instance: &SandboxInstance,
        path: &str,
        content: &[u8],
    ) -> Result<()> {
        let state = self.get_state(&instance.id)?;
        if state.spec.read_only {
            return Err(Error::Sandbox(format!(
                "write_file '{}': sandbox is read-only",
                path
            )));
        }
        let resolved = resolve_sandbox_path(state.dir.path(), path)?;
        if let Some(parent) = resolved.parent() {
            tokio::fs::create_dir_all(parent)
                .await
                .map_err(|e| Error::Sandbox(format!("create_dir_all '{}': {e}", path)))?;
        }
        tokio::fs::write(&resolved, content)
            .await
            .map_err(|e| Error::Sandbox(format!("write_file '{}': {e}", path)))
    }

    async fn list_changes(&self, instance: &SandboxInstance) -> Result<Vec<FileChange>> {
        let state = self.get_state(&instance.id)?;
        let mut changes = Vec::new();
        collect_changes(state.dir.path(), state.dir.path(), &mut changes)
            .map_err(|e| Error::Sandbox(format!("list_changes: {e}")))?;
        Ok(changes)
    }

    async fn destroy(&self, instance: &SandboxInstance) -> Result<()> {
        let mut guard = self
            .instances
            .lock()
            .map_err(|e| Error::Sandbox(format!("ProcessBackend instance lock poisoned: {e}")))?;
        guard.remove(&instance.id);
        Ok(())
    }
}

fn collect_changes(base: &Path, current: &Path, out: &mut Vec<FileChange>) -> std::io::Result<()> {
    let entries = std::fs::read_dir(current)?;
    for entry in entries {
        let entry = entry?;
        let path = entry.path();
        let relative = path
            .strip_prefix(base)
            .map_err(|e| std::io::Error::other(e.to_string()))?
            .to_string_lossy()
            .to_string();
        if path.is_dir() {
            collect_changes(base, &path, out)?;
        } else {
            out.push(FileChange {
                path: relative,
                kind: FileChangeKind::Created,
            });
        }
    }
    Ok(())
}

#[cfg(unix)]
fn apply_unix_limits(command: &mut tokio::process::Command, spec: &SandboxSpec) {
    let cpu_soft = spec.max_cpu_seconds.unwrap_or(300);
    let cpu_hard = cpu_soft * 2;
    let mem_bytes = spec
        .max_memory_mb
        .unwrap_or(1024)
        .saturating_mul(1024 * 1024);
    let mem_hard = mem_bytes.saturating_mul(2);

    unsafe {
        command.pre_exec(move || {
            let _ = nix::sys::resource::setrlimit(
                nix::sys::resource::Resource::RLIMIT_CPU,
                cpu_soft,
                cpu_hard,
            );
            let _ = nix::sys::resource::setrlimit(
                nix::sys::resource::Resource::RLIMIT_AS,
                mem_bytes,
                mem_hard,
            );
            Ok(())
        });
    }
}

/// A `ProcessHandle` backed by a `tokio::process::Child`.
pub struct TokioProcessHandle {
    child: Arc<TokioMutex<Child>>,
    stdout: Arc<TokioMutex<Option<ChildStdout>>>,
    stderr: Arc<TokioMutex<Option<ChildStderr>>>,
    #[allow(dead_code)]
    state: Arc<InstanceState>,
    timeout_seconds: Option<u64>,
}

impl TokioProcessHandle {
    fn new(mut child: Child, state: Arc<InstanceState>, timeout_seconds: Option<u64>) -> Self {
        let stdout = child.stdout.take();
        let stderr = child.stderr.take();
        Self {
            child: Arc::new(TokioMutex::new(child)),
            stdout: Arc::new(TokioMutex::new(stdout)),
            stderr: Arc::new(TokioMutex::new(stderr)),
            state,
            timeout_seconds,
        }
    }

    fn line_stream(
        reader: Option<impl tokio::io::AsyncRead + Unpin + Send + 'static>,
    ) -> LineStream {
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
        Box::pin(
            futures::stream::once(async move {
                let mut guard = stdout.lock().await;
                guard.take()
            })
            .flat_map(Self::line_stream),
        )
    }

    fn stderr(&self) -> LineStream {
        let stderr = self.stderr.clone();
        Box::pin(
            futures::stream::once(async move {
                let mut guard = stderr.lock().await;
                guard.take()
            })
            .flat_map(Self::line_stream),
        )
    }

    async fn kill(&self) -> Result<()> {
        let mut child = self.child.lock().await;
        child.kill().await.map_err(Error::Io)
    }

    async fn wait(&self) -> Result<std::process::ExitStatus> {
        let mut child = self.child.lock().await;
        let fut = child.wait();
        match self.timeout_seconds {
            Some(sec) => {
                let timeout = tokio::time::Duration::from_secs(sec);
                tokio::time::timeout(timeout, fut)
                    .await
                    .map_err(|_| Error::Sandbox("process timed out".into()))?
                    .map_err(Error::Io)
            }
            None => fut.await.map_err(Error::Io),
        }
    }
}

/// Resolve a relative path against a base directory and reject traversal
/// outside the base.
pub fn resolve_sandbox_path(base: &Path, path: &str) -> Result<PathBuf> {
    if path.starts_with('/') {
        return Err(Error::Sandbox(format!(
            "absolute paths are not allowed: {}",
            path
        )));
    }

    let base_canonical = base.canonicalize().unwrap_or_else(|_| base.to_path_buf());

    let mut resolved = PathBuf::new();
    for component in path.split('/') {
        match component {
            "" | "." => continue,
            ".." => {
                if !resolved.pop() {
                    return Err(Error::Sandbox(format!(
                        "path '{}' escapes sandbox root",
                        path
                    )));
                }
            }
            other => resolved.push(other),
        }
    }

    let joined = base_canonical.join(&resolved);
    if !joined.starts_with(&base_canonical) {
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
    use hnsx_core::sandbox::{SandboxPolicy, SandboxRuntime, SandboxSpec};

    fn backend() -> ProcessBackend {
        ProcessBackend::new()
    }

    fn spec() -> SandboxSpec {
        SandboxSpec {
            policy: SandboxPolicy::Process,
            runtime: SandboxRuntime::Process,
            ..Default::default()
        }
    }

    #[tokio::test]
    async fn echo_command_streams_stdout() {
        let backend = backend();
        let instance = backend.create(&spec()).await.expect("create");
        let handle = backend
            .execute(&instance, "echo hello world", &HashMap::new())
            .await
            .expect("execute");
        let mut stdout = handle.stdout();
        let line = stdout.next().await.expect("a line").expect("ok line");
        assert_eq!(line, "hello world");
        assert!(stdout.next().await.is_none());
    }

    #[tokio::test]
    async fn stderr_is_separate_stream() {
        let backend = backend();
        let instance = backend.create(&spec()).await.expect("create");
        let handle = backend
            .execute(&instance, "echo err >&2", &HashMap::new())
            .await
            .expect("execute");
        let mut stderr = handle.stderr();
        let line = stderr.next().await.expect("a line").expect("ok line");
        assert_eq!(line, "err");
    }

    #[tokio::test]
    async fn env_is_passed_to_command() {
        let backend = backend();
        let instance = backend.create(&spec()).await.expect("create");
        let mut env = HashMap::new();
        env.insert("FOO".to_string(), "bar".to_string());
        let handle = backend
            .execute(&instance, "echo $FOO", &env)
            .await
            .expect("execute");
        let mut stdout = handle.stdout();
        let line = stdout.next().await.expect("a line").expect("ok line");
        assert_eq!(line, "bar");
    }

    #[tokio::test]
    async fn write_file_and_read_file_round_trip() {
        let backend = backend();
        let instance = backend.create(&spec()).await.expect("create");
        backend
            .write_file(&instance, "subdir/test.txt", b"hello")
            .await
            .expect("write");
        let content = backend
            .read_file(&instance, "subdir/test.txt")
            .await
            .expect("read");
        assert_eq!(content, b"hello");
    }

    #[tokio::test]
    async fn list_changes_detects_new_files() {
        let backend = backend();
        let instance = backend.create(&spec()).await.expect("create");
        backend
            .write_file(&instance, "a.txt", b"a")
            .await
            .expect("write");
        backend
            .write_file(&instance, "dir/b.txt", b"b")
            .await
            .expect("write");
        let changes = backend.list_changes(&instance).await.expect("list");
        let paths: Vec<_> = changes.iter().map(|c| c.path.as_str()).collect();
        assert!(paths.contains(&"a.txt"));
        assert!(paths.contains(&"dir/b.txt"));
    }

    #[tokio::test]
    async fn read_file_rejects_escape_attempt() {
        let backend = backend();
        let instance = backend.create(&spec()).await.expect("create");
        let err = backend
            .read_file(&instance, "../etc/passwd")
            .await
            .unwrap_err();
        let msg = format!("{err}");
        assert!(msg.contains("escapes"), "got: {msg}");
    }

    #[tokio::test]
    async fn execute_times_out_when_limit_exceeded() {
        let backend = backend();
        let mut spec = spec();
        spec.timeout_seconds = Some(1);
        let instance = backend.create(&spec).await.expect("create");
        let handle = backend
            .execute(&instance, "sleep 10", &HashMap::new())
            .await
            .expect("execute");
        let err = handle.wait().await.unwrap_err();
        assert!(format!("{err}").contains("timed out"), "got: {err:?}");
    }
}
