#![allow(dead_code)]

//! HnsX control plane. Skeleton only — implementation lands in Phase 2
//! alongside the gRPC service. See Initial_Architectrue.md §5.3.

pub mod discovery;
pub mod registry;
pub mod scheduler;
pub mod telemetry;
