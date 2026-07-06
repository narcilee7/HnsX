#![allow(dead_code)] // skeleton phase: trait stubs are intentionally unused

//! HnsX core runtime: shared traits (Domain, Agent, Adapter, Sandbox, MemoryBackend),
//! cross-cutting types (Chunk, InvokeContext, RuntimeContext), and error type.
//!
//! See `design/Tech/V1/Initial_Architectrue.md` for the full architecture.

pub mod adapter;
pub mod agent;
pub mod agent_factory;
pub mod bus;
pub mod chunk;
pub mod domain;
pub mod error;
pub mod loader;
pub mod memory;
pub mod noop;
pub mod pii;
pub mod reporter;
pub mod sandbox;
pub mod telemetry;
pub mod tool;
pub mod workflow;

pub use adapter::{Adapter, RuntimeContext};
pub use agent::{
    AdapterConfig, Agent, AgentSchema, AgentSpec, HealthStatus, InvokeContext, ModelRef,
    PromptTemplate, Provider, ToolKind, ToolRef,
};
pub use agent_factory::{AgentFactory, NoopFactory};
pub use chunk::{Artifact, Chunk, FileChange, FileChangeKind};
pub use domain::{Domain, DomainSpec, Step, Workflow};
pub use error::{Error, Result};
pub use loader::{DomainLoader, LoadedDomain};
pub use memory::{
    InMemoryBackend, MemoryBackend, MemoryBackendFactory, MemoryConfig, Message, PostgresBackend,
    RedisBackend, Session, SqliteBackend, Turn,
};
pub use noop::NoopAgent;
pub use pii::{contains_pii, detect};
pub use reporter::{GrpcReporter, NoopReporter, Reporter, SharedReporter};
pub use sandbox::{LineStream, ProcessHandle, Sandbox, SandboxInstance, SandboxPolicy, SandboxRuntime, SandboxSpec};
pub use telemetry::{StepTrace, Telemetry, now_ms};
pub use tool::{Tool, ToolRegistry, ToolSpec};
