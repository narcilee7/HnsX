#![allow(dead_code)]

//! HnsX adapter layer.
//!
//! Each adapter module wraps an external AI agent provider behind the
//! `hnsx_core::Adapter` trait. See Initial_Architectrue.md §4.3.

pub mod anthropic;
pub mod claude_code;
pub mod codex;
pub mod custom;
pub mod genai;
pub mod tools;
pub mod openai;

pub use claude_code::ClaudeCodeAgent;
pub use genai::{GenaiAgent, GenaiAgentFactory};
pub use hnsx_core::{Adapter, AdapterConfig, AgentFactory, RuntimeContext};

use std::sync::Arc;

use hnsx_core::agent::{AgentSpec, Provider};
use hnsx_core::agent_factory::AgentFactory as CoreAgentFactory;
use hnsx_core::Agent;
use hnsx_core::Result;
use hnsx_sandbox::factory::SandboxFactory;

/// Factory that resolves a `Provider` to a concrete `Agent` impl.
///
/// - HTTP providers (OpenAI, Anthropic, Ollama, Custom, Codex) go through
///   the multi-provider `genai` client.
/// - `claude-code` spawns the local Claude Code CLI inside a sandbox.
#[derive(Clone, Default)]
pub struct HnsxAgentFactory {
    sandbox_factory: Arc<SandboxFactory>,
}

impl HnsxAgentFactory {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn with_sandbox_factory(sandbox_factory: Arc<SandboxFactory>) -> Self {
        Self { sandbox_factory }
    }
}

impl CoreAgentFactory for HnsxAgentFactory {
    fn create(&self, spec: &AgentSpec) -> Result<Arc<dyn Agent>> {
        match spec.model.provider {
            Provider::ClaudeCode => {
                let sandbox_spec = spec.sandbox.clone().unwrap_or(hnsx_core::sandbox::SandboxSpec {
                    policy: hnsx_core::sandbox::SandboxPolicy::Namespace,
                    runtime: hnsx_core::sandbox::SandboxRuntime::Auto,
                });
                let sandbox = self.sandbox_factory.create(&sandbox_spec);
                Ok(Arc::new(ClaudeCodeAgent::new(sandbox, spec)))
            }
            _ => GenaiAgentFactory::default().create(spec),
        }
    }
}

/// Backwards-compatible alias.
pub type AdapterFactory = HnsxAgentFactory;
