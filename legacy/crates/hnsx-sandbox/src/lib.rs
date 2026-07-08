#![allow(dead_code)]

//! HnsX sandbox layer.
//!
//! This crate provides platform-specific backends for `hnsx_core::Sandbox`.
//! The goal is a single cross-platform isolation contract: the same
//! `SandboxPolicy` produces equivalent security outcomes on local laptops
//! (macOS / Linux / Windows) and in the cloud (containers / micro-VMs).
//!
//! Backends
//! --------
//! - `none`      : no isolation (default for pure network adapters)
//! - `process`   : process-level hardening available on every OS
//! - `linux`     : namespaces + landlock + seccomp + cgroups v2
//! - `macos`     : seatbelt / posix_spawn / rlimit (process-hardening subset)
//! - `windows`   : job objects / ACLs
//! - `container` : OCI runtime (Docker, containerd, podman)
//! - `microvm`   : Firecracker / Kata / Cloud Hypervisor
//!
//! The `auto` runtime in `SandboxSpec` maps a policy to the best available
//! backend for the current platform or deployment target.

pub mod backend;
pub mod factory;

pub use hnsx_core::Sandbox;
