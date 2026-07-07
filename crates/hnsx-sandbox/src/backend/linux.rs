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
use std::path::PathBuf;
use std::process::Stdio;
use std::sync::Arc;

use async_trait::async_trait;
use futures::StreamExt;
use hnsx_core::{
    chunk::FileChange,
    error::{Error, Result},
    sandbox::{LineStream, ProcessHandle, Sandbox, SandboxInstance, SandboxSpec},
};
use nix::sched::{CloneFlags, unshare};
use nix::unistd::chroot;
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, ChildStderr, ChildStdout};
use tokio::sync::Mutex;

pub struct LinuxNamespaceBackend;

impl LinuxNamespaceBackend {
    pub fn new() -> Self {
        Self
    }
}

impl Default for LinuxNamespaceBackend {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl Sandbox for LinuxNamespaceBackend {
    async fn create(&self, _spec: &SandboxSpec) -> Result<SandboxInstance> {
        Ok(SandboxInstance {
            id: "linux-namespace".to_string(),
        })
    }

    async fn execute(
        &self,
        cmd: &str,
        env: &HashMap<String, String>,
    ) -> Result<Box<dyn ProcessHandle>> {
        let root_dir = tempfile::tempdir()
            .map_err(|e| Error::Sandbox(format!("LinuxNamespaceBackend rootdir: {e}")))?;
        let root_path = root_dir.path().to_path_buf();

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

                chroot(&root_for_pre_exec
                    .to_str()
                    .ok_or_else(|| std::io::Error::new(
                        std::io::ErrorKind::Other,
                        "non-utf8 chroot path",
                    ))?,
                )
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

        Ok(Box::new(LinuxProcessHandle::new(child, root_dir)))
    }

    async fn read_file(&self, _path: &str) -> Result<Vec<u8>> {
        Err(Error::Unimplemented("LinuxNamespaceBackend::read_file"))
    }

    async fn write_file(
        &self,
        _path: &str,
        _content: &[u8],
    ) -> Result<()> {
        Err(Error::Unimplemented("LinuxNamespaceBackend::write_file"))
    }

    async fn list_changes(&self) -> Result<Vec<FileChange>> {
        Ok(Vec::new())
    }

    async fn destroy(&self) -> Result<()> {
        Ok(())
    }
}

struct LinuxProcessHandle {
    child: Arc<Mutex<Child>>,
    stdout: Arc<Mutex<Option<ChildStdout>>>,
    stderr: Arc<Mutex<Option<ChildStderr>>>,
    _root_dir: tempfile::TempDir,
}

impl LinuxProcessHandle {
    fn new(mut child: Child, root_dir: tempfile::TempDir) -> Self {
        let stdout = child.stdout.take();
        let stderr = child.stderr.take();
        Self {
            child: Arc::new(Mutex::new(child)),
            stdout: Arc::new(Mutex::new(stdout)),
            stderr: Arc::new(Mutex::new(stderr)),
            _root_dir: root_dir,
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
impl ProcessHandle for LinuxProcessHandle {
    fn stdout(&self) -> LineStream {
        let stdout = self.stdout.clone();
        Box::pin(futures::stream::once(async move {
            let mut guard = stdout.lock().await;
            guard.take()
        })
        .flat_map(|reader| Self::line_stream(reader)))
    }

    fn stderr(&self) -> LineStream {
        let stderr = self.stderr.clone();
        Box::pin(futures::stream::once(async move {
            let mut guard = stderr.lock().await;
            guard.take()
        })
        .flat_map(|reader| Self::line_stream(reader)))
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
        };

        // Write a file inside the sandbox.
        let handle = backend
            .execute("echo secret > /tmp/sandbox-write.txt", &HashMap::new())
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
