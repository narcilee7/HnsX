#![allow(dead_code)]

//! HnsX control plane. Implements Registry, Scheduler, Discovery and
//! Telemetry gRPC services on top of shared SQLite state.

pub mod discovery;
pub mod http_api;
pub mod metrics;
pub mod proto;
pub mod registry;
pub mod scheduler;
pub mod server;
pub mod store;
pub mod telemetry;

pub use server::ControlPlaneServer;
