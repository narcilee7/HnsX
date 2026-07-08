//! Linux namespace backend.
//!
//! Provides OS-level isolation via Linux namespaces. The first implementation
//! is intentionally minimal: a new mount namespace + chroot into a disposable
//! root filesystem. This is enough to prove that writes inside the sandbox do
//! not leak to the host.
//!
//! Later Phase 2.x iterations add:
//! - landlock filesystem rules
//! - seccomp syscall filtering
//! - cgroups v2 resource quotas
//! - network namespace + veth / loopback

use std::collections::HashMap;
use std::os::unix::process::ExitStatusExt;
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
use nix::sched::{CloneFlags, unshare};
use nix::unistd::chroot;
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, ChildStderr, ChildStdout};
use tokio::sync::Mutex as TokioMutex;

pub struct LinuxNamespaceBackend {
    instances: Mutex<HashMap<String, Arc<InstanceState>>>,
}

struct InstanceState {
    dir: Arc<tempfile::TempDir>,
    spec: SandboxSpec,
}

impl LinuxNamespaceBackend {
    pub fn new() -> Self {
        Self {
            instances: Mutex::new(HashMap::new()),
        }
    }

    fn get_state(&self, id: &str) -> Result<Arc<InstanceState>> {
        let guard = self.instances.lock().map_err(|e| {
            Error::Sandbox(format!("LinuxNamespaceBackend instance lock poisoned: {e}"))
        })?;
        guard
            .get(id)
            .cloned()
            .ok_or_else(|| Error::Sandbox(format!("sandbox instance {id} not found")))
    }
}

impl Default for LinuxNamespaceBackend {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl Sandbox for LinuxNamespaceBackend {
    async fn create(&self, spec: &SandboxSpec) -> Result<SandboxInstance> {
        let id = uuid::Uuid::new_v4().to_string();
        let dir = Arc::new(
            tempfile::tempdir()
                .map_err(|e| Error::Sandbox(format!("LinuxNamespaceBackend rootdir: {e}")))?,
        );

        {
            let mut guard = self.instances.lock().map_err(|e| {
                Error::Sandbox(format!("LinuxNamespaceBackend instance lock poisoned: {e}"))
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
        let root_path = state.dir.path().to_path_buf();
        let spec = state.spec.clone();

        // Populate the chroot with a minimal /tmp so tools can write there.
        let tmp = root_path.join("tmp");
        std::fs::create_dir_all(&tmp)
            .map_err(|e| Error::Sandbox(format!("create /tmp in chroot: {e}")))?;

        let mut command = tokio::process::Command::new("sh");
        command.arg("-c").arg(cmd);
        command
            .env_clear()
            .envs(env)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped());

        // Keep PATH so that installed CLIs remain resolvable.
        if let Ok(path) = std::env::var("PATH") {
            command.env("PATH", path);
        }

        let root_for_pre_exec = root_path.clone();
        unsafe {
            command.pre_exec(move || {
                // New mount namespace: subsequent mounts/chroot do not affect
                // the host mount table.
                unshare(CloneFlags::CLONE_NEWNS).map_err(|e| {
                    std::io::Error::new(std::io::ErrorKind::Other, format!("unshare: {e}"))
                })?;

                // Make the mount namespace private so that mounts inside the
                // chroot do not propagate out.
                nix::mount::mount(
                    Some("/"),
                    "/",
                    Option::<&str>::None,
                    nix::mount::MsFlags::MS_REC | nix::mount::MsFlags::MS_PRIVATE,
                    Option::<&str>::None,
                )
                .map_err(|e| {
                    std::io::Error::new(std::io::ErrorKind::Other, format!("mount private: {e}"))
                })?;

                chroot(root_for_pre_exec.to_str().ok_or_else(|| {
                    std::io::Error::new(std::io::ErrorKind::Other, "non-utf8 chroot path")
                })?)
                .map_err(|e| {
                    std::io::Error::new(std::io::ErrorKind::Other, format!("chroot: {e}"))
                })?;

                std::env::set_current_dir("/").map_err(|e| {
                    std::io::Error::new(std::io::ErrorKind::Other, format!("chdir: {e}"))
                })?;

                Ok(())
            });
        }

        let child = command
            .spawn()
            .map_err(|e| Error::Sandbox(format!("LinuxNamespaceBackend spawn '{cmd}': {e}")))?;

        Ok(Box::new(LinuxProcessHandle::new(
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
        let mut guard = self.instances.lock().map_err(|e| {
            Error::Sandbox(format!("LinuxNamespaceBackend instance lock poisoned: {e}"))
        })?;
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

struct LinuxProcessHandle {
    child: Arc<TokioMutex<Child>>,
    stdout: Arc<TokioMutex<Option<ChildStdout>>>,
    stderr: Arc<TokioMutex<Option<ChildStderr>>>,
    #[allow(dead_code)]
    state: Arc<InstanceState>,
    timeout_seconds: Option<u64>,
}

impl LinuxProcessHandle {
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
impl ProcessHandle for LinuxProcessHandle {
    fn stdout(&self) -> LineStream {
        let stdout = self.stdout.clone();
        Box::pin(
            futures::stream::once(async move {
                let mut guard = stdout.lock().await;
                guard.take()
            })
            .flat_map(|reader| Self::line_stream(reader)),
        )
    }

    fn stderr(&self) -> LineStream {
        let stderr = self.stderr.clone();
        Box::pin(
            futures::stream::once(async move {
                let mut guard = stderr.lock().await;
                guard.take()
            })
            .flat_map(|reader| Self::line_stream(reader)),
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

fn resolve_sandbox_path(base: &Path, path: &str) -> Result<PathBuf> {
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

#[cfg(all(test, target_os = "linux"))]
mod tests {
    use super::*;
    use futures::StreamExt;

    #[tokio::test]
    async fn chroot_isolates_writes_from_host() {
        let backend = LinuxNamespaceBackend::new();
        let spec = SandboxSpec {
            policy: hnsx_core::sandbox::SandboxPolicy::Namespace,
            runtime: hnsx_core::sandbox::SandboxRuntime::LinuxNamespace,
            ..Default::default()
        };
        let instance = backend.create(&spec).await.expect("create");

        // Write a file inside the sandbox.
        let handle = backend
            .execute(
                &instance,
                "echo secret > /tmp/sandbox-write.txt",
                &HashMap::new(),
            )
            .await
            .expect("execute");
        let _ = handle.wait().await;

        // The file must NOT exist on the host filesystem.
        assert!(
            !std::path::Path::new("/tmp/sandbox-write.txt").exists(),
            "sandbox write leaked to host /tmp"
        );
    }
}
