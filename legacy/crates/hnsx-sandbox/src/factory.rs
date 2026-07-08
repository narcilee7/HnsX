//! Sandbox factory: maps a `SandboxSpec` to the best available backend.

use std::sync::Arc;

use hnsx_core::Sandbox;
use hnsx_core::sandbox::{SandboxPolicy, SandboxRuntime, SandboxSpec};

use crate::backend::{none::NoneBackend, process::ProcessBackend};

#[cfg(target_os = "linux")]
use crate::backend::linux::LinuxNamespaceBackend;

#[derive(Debug, Clone, Default)]
pub struct SandboxFactory;

impl SandboxFactory {
    pub fn new() -> Self {
        Self
    }

    /// Select and instantiate a backend for `spec`.
    pub fn create(&self, spec: &SandboxSpec) -> Arc<dyn Sandbox> {
        let runtime = resolve_runtime(spec);

        match runtime {
            SandboxRuntime::None | SandboxRuntime::Auto if spec.policy == SandboxPolicy::None => {
                Arc::new(NoneBackend)
            }
            SandboxRuntime::Process | SandboxRuntime::Auto => Arc::new(ProcessBackend::new()),
            #[cfg(target_os = "linux")]
            SandboxRuntime::LinuxNamespace => Arc::new(LinuxNamespaceBackend::new()),
            #[cfg(not(target_os = "linux"))]
            SandboxRuntime::LinuxNamespace => {
                // Fall back to process-level hardening on non-Linux platforms.
                Arc::new(ProcessBackend::new())
            }
            // Fallback to process-level hardening when a stronger backend is
            // requested but not compiled in for this platform.
            _ => Arc::new(ProcessBackend::new()),
        }
    }
}

fn resolve_runtime(spec: &SandboxSpec) -> SandboxRuntime {
    if spec.runtime != SandboxRuntime::Auto {
        return spec.runtime;
    }

    match spec.policy {
        SandboxPolicy::None => SandboxRuntime::None,
        SandboxPolicy::Process => SandboxRuntime::Process,
        SandboxPolicy::Namespace => {
            #[cfg(target_os = "linux")]
            {
                SandboxRuntime::LinuxNamespace
            }
            #[cfg(not(target_os = "linux"))]
            {
                SandboxRuntime::Process
            }
        }
        SandboxPolicy::Container => SandboxRuntime::Container,
        SandboxPolicy::Vm => SandboxRuntime::MicroVm,
    }
}
