//! No-op / none backend.
//!
//! Used when `SandboxPolicy::None` is requested, or when no other backend is
//! available and the caller explicitly opts out of isolation.

use std::collections::HashMap;

use async_trait::async_trait;
use hnsx_core::{
    chunk::FileChange,
    error::{Error, Result},
    sandbox::{ProcessHandle, Sandbox, SandboxInstance, SandboxSpec},
};

pub struct NoneBackend;

#[async_trait]
impl Sandbox for NoneBackend {
    async fn create(&self, _spec: &SandboxSpec) -> Result<SandboxInstance> {
        Ok(SandboxInstance {
            id: "none".to_string(),
        })
    }

    async fn execute(
        &self,
        _cmd: &str,
        _env: &HashMap<String, String>,
    ) -> Result<Box<dyn ProcessHandle>> {
        Err(Error::Unimplemented("NoneBackend::execute"))
    }

    async fn read_file(&self, _path: &str) -> Result<Vec<u8>> {
        Err(Error::Unimplemented("NoneBackend::read_file"))
    }

    async fn write_file(&self, _path: &str, _content: &[u8]) -> Result<()> {
        Err(Error::Unimplemented("NoneBackend::write_file"))
    }

    async fn list_changes(&self) -> Result<Vec<FileChange>> {
        Ok(Vec::new())
    }

    async fn destroy(&self) -> Result<()> {
        Ok(())
    }
}
