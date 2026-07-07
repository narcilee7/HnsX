#![allow(dead_code)]

//! HnsX adapter layer.
//!
//! Each adapter module wraps an external AI agent provider behind the
//! `hnsx_core::Adapter` trait. See Initial_Architectrue.md §4.3.

pub mod anthropic;
pub mod anthropic_adapter;
pub mod claude_code;
pub mod codex;
pub mod custom;
pub mod custom_adapter;
pub mod genai;
pub mod genai_adapter;
pub mod http_common;
pub mod ollama;
pub mod ollama_adapter;
pub mod openai;
pub mod openai_adapter;
pub mod tool_chat;
pub mod tools;

pub use anthropic_adapter::AnthropicAdapter;
pub use claude_code::ClaudeCodeAdapter;
pub use codex::CodexAdapter;
pub use custom_adapter::CustomAdapter;
pub use genai::{GenaiAgent, GenaiAgentFactory};
pub use genai_adapter::{GenaiAdapter, GenaiAdapterFactory};
pub use hnsx_core::{Adapter, AdapterConfig, AgentFactory, RuntimeContext};
pub use ollama_adapter::OllamaAdapter;
pub use openai_adapter::OpenAIAdapter;

use std::sync::Arc;

use hnsx_core::agent::{AgentSpec, Provider};
use hnsx_core::agent_factory::AgentFactory as CoreAgentFactory;
use hnsx_core::sandbox::{SandboxPolicy, SandboxRuntime, SandboxSpec};
use hnsx_core::{Agent, HnsXAgentBuilder};
use hnsx_core::Result;
use hnsx_sandbox::factory::SandboxFactory;

/// Factory that resolves a `Provider` to a concrete `Agent` impl.
///
/// All providers are ultimately composed into an `HnsXAgent` via
/// `HnsXAgentBuilder`. HTTP providers use native `reqwest`-based adapters with
/// native tool-call support.
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
        let tool_registry = crate::tools::build_tool_registry(&spec.tools).unwrap_or_default();

        let adapter: Arc<dyn Adapter> = match spec.model.provider {
            Provider::Openai => Arc::new(
                crate::openai_adapter::OpenAIAdapter::new(spec)?.with_tools(tool_registry.clone()),
            ),
            Provider::Anthropic => Arc::new(
                crate::anthropic_adapter::AnthropicAdapter::new(spec)?
                    .with_tools(tool_registry.clone()),
            ),
            Provider::Ollama => Arc::new(
                crate::ollama_adapter::OllamaAdapter::new(spec)?.with_tools(tool_registry.clone()),
            ),
            Provider::Custom => Arc::new(
                crate::custom_adapter::CustomAdapter::new(spec)?.with_tools(tool_registry.clone()),
            ),
            Provider::ClaudeCode => Arc::new(
                crate::claude_code::ClaudeCodeAdapter::new(sandbox.clone(), spec)
                    .with_tools(tool_registry.clone()),
            ),
            Provider::Codex => Arc::new(
                crate::codex::CodexAdapter::new(sandbox.clone(), spec)
                    .with_tools(tool_registry.clone()),
            ),
        };

        Ok(Arc::new(
            HnsXAgentBuilder::new()
                .spec(spec.clone())
                .adapter(adapter)
                .sandbox(sandbox)
                .tools(tool_registry)
                .build()?,
        ))
    }
}

/// Backwards-compatible alias.
pub type AdapterFactory = HnsxAgentFactory;
