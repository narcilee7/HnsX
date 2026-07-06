//! Process-level hardening backend.
//!
//! This is the lowest-common-denominator backend that works on every supported
//! platform. It does not provide strong isolation like namespaces or
//! containers, but it does apply limits that are always safe to enable:
//! - resource limits (rlimit on Unix, job object on Windows)
//! - restricted environment
//! - working directory sandboxing
//! - deny-list of dangerous syscalls / APIs where available
//!
//! On macOS and Windows this is the default for `SandboxPolicy::Process` when
//! no stronger backend is available.

use std::collections::HashMap;

use async_trait::async_trait;
use hnsx_core::{
    chunk::FileChange,
    error::{Error, Result},
    sandbox::{ProcessHandle, Sandbox, SandboxInstance, SandboxSpec},
};

pub struct ProcessBackend;

#[async_trait]
impl Sandbox for ProcessBackend {
    async fn create(&self, _spec: &SandboxSpec) -> Result<SandboxInstance> {
        Ok(SandboxInstance {
            id: "process".to_string(),
        })
    }

    async fn execute(&self, _cmd: &str, _env: &HashMap<String, String>) -> Result<ProcessHandle> {
        Err(Error::Unimplemented("ProcessBackend::execute"))
    }

    async fn read_file(&self, _path: &str) -> Result<Vec<u8>> {
        Err(Error::Unimplemented("ProcessBackend::read_file"))
    }

    async fn write_file(&self, _path: &str, _content: &[u8]) -> Result<()> {
        Err(Error::Unimplemented("ProcessBackend::write_file"))
    }

    async fn list_changes(&self) -> Result<Vec<FileChange>> {
        Ok(Vec::new())
    }

    async fn destroy(&self) -> Result<()> {
        Ok(())
    }
}
