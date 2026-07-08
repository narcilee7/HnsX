//! `AgentFactory`: turns an `AgentSpec` into a runnable `Arc<dyn Agent>`.
//!
//! Phase 1.4 introduces this indirection so the loader can swap noop agents
//! for real adapters (1.4-1.5) without changing its public surface.

use std::sync::Arc;

use crate::agent::{Agent, AgentSpec};
use crate::error::Result;
use crate::noop::NoopAgent;

/// Builds an `Arc<dyn Agent>` from an `AgentSpec`.
///
/// Implementations live in `hnsx-adapter` (e.g. `GenaiAgentFactory`).
/// The default [`NoopFactory`] is what the loader uses when no real factory
/// is configured.
pub trait AgentFactory: Send + Sync {
    fn create(&self, spec: &AgentSpec) -> Result<Arc<dyn Agent>>;
}

/// Default factory: returns a [`NoopAgent`] for every spec. Used for local
/// development, CI, and any environment where LLM credentials are not
/// available.
#[derive(Debug, Default, Clone, Copy)]
pub struct NoopFactory;

impl AgentFactory for NoopFactory {
    fn create(&self, _spec: &AgentSpec) -> Result<Arc<dyn Agent>> {
        Ok(NoopAgent::arc())
    }
}
