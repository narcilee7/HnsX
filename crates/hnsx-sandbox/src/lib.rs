#![allow(dead_code)]

//! HnsX sandbox layer.
//!
//! Real implementations of `hnsx_core::Sandbox` are Linux-only (namespace +
//! landlock + seccomp + cgroups). On other platforms the crate compiles to
//! an empty module so the workspace can be developed on macOS / Windows.

#[cfg(target_os = "linux")]
pub mod linux;

pub use hnsx_core::Sandbox;
