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
pub mod genai_adapter;
pub mod tools;
pub mod openai;

pub use claude_code::ClaudeCodeAgent;
pub use codex::CodexAgent;
pub use genai::{GenaiAgent, GenaiAgentFactory};
pub use genai_adapter::{GenaiAdapter, GenaiAdapterFactory};
pub use hnsx_core::{Adapter, AdapterConfig, AgentFactory, RuntimeContext};

use std::sync::Arc;

use hnsx_core::agent::{AgentSpec, Provider};
use hnsx_core::agent_factory::AgentFactory as CoreAgentFactory;
use hnsx_core::sandbox::{SandboxPolicy, SandboxRuntime, SandboxSpec};
use hnsx_core::{Agent, HnsXAgentBuilder};
use hnsx_core::Result;
use hnsx_sandbox::factory::SandboxFactory;

/// Factory that resolves a `Provider` to a concrete `Agent` impl.
///
/// - HTTP providers (OpenAI, Anthropic, Ollama, Custom) go through the
///   multi-provider `genai` client.
/// - `claude-code` spawns the local Claude Code CLI inside a sandbox.
/// - `codex` spawns the local Codex CLI inside a sandbox.
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
        let sandbox_spec = spec.sandbox.clone().unwrap_or(SandboxSpec {
            policy: SandboxPolicy::None,
            runtime: SandboxRuntime::Auto,
        });
        let sandbox = self.sandbox_factory.create(&sandbox_spec);

        match spec.model.provider {
            Provider::ClaudeCode | Provider::Codex => {
                let inner: Arc<dyn Agent> = match spec.model.provider {
                    Provider::ClaudeCode => Arc::new(crate::claude_code::ClaudeCodeAgent::new(sandbox.clone(), spec)),
                    Provider::Codex => Arc::new(crate::codex::CodexAgent::new(sandbox.clone(), spec)),
                    _ => unreachable!(),
                };
                Ok(inner)
            }
            _ => {
                let adapter = crate::genai_adapter::GenaiAdapterFactory::default().create_adapter(spec)?;
                Ok(Arc::new(
                    HnsXAgentBuilder::new()
                        .spec(spec.clone())
                        .adapter(adapter)
                        .sandbox(sandbox)
                        .tools(crate::tools::build_tool_registry(&spec.tools).unwrap_or_default())
                        .build()?,
                ))
            }
        }
    }
}

/// Backwards-compatible alias.
pub type AdapterFactory = HnsxAgentFactory;
