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

use hnsx_core::Result;
use hnsx_core::agent::{AgentSpec, Provider};
use hnsx_core::agent_factory::AgentFactory as CoreAgentFactory;
use hnsx_core::sandbox::{SandboxPolicy, SandboxRuntime, SandboxSpec};
use hnsx_core::{Agent, HnsXAgentBuilder};
use hnsx_sandbox::factory::SandboxFactory;

/// Secret resolver so `HnsxAgentFactory` can fill in missing API keys without
/// depending on `hnsx-cli`.
pub trait SecretResolver: Send + Sync {
    /// Return the API key for a provider, optionally scoped to an agent id.
    fn resolve(&self, provider: &str, agent_id: &str) -> Option<String>;
}

/// Environment-variable helper used by the factory and CLI.
pub fn api_key_env_var(provider: &str) -> Option<&'static str> {
    match provider {
        "openai" => Some("OPENAI_API_KEY"),
        "anthropic" => Some("ANTHROPIC_API_KEY"),
        "custom" => Some("CUSTOM_API_KEY"),
        _ => None,
    }
}

/// Factory that resolves a `Provider` to a concrete `Agent` impl.
///
/// All providers are ultimately composed into an `HnsXAgent` via
/// `HnsXAgentBuilder`. HTTP providers use native `reqwest`-based adapters with
/// native tool-call support.
#[derive(Clone, Default)]
pub struct HnsxAgentFactory {
    sandbox_factory: Arc<SandboxFactory>,
    secrets: Option<Arc<dyn SecretResolver>>,
    api_key_override: Option<String>,
}

impl HnsxAgentFactory {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn with_sandbox_factory(sandbox_factory: Arc<SandboxFactory>) -> Self {
        Self {
            sandbox_factory,
            ..Default::default()
        }
    }

    /// Attach a secret resolver so the factory can fill in missing API keys.
    pub fn with_secret_resolver(mut self, resolver: Arc<dyn SecretResolver>) -> Self {
        self.secrets = Some(resolver);
        self
    }

    /// Override the API key used for all HTTP providers. Useful for CLI flags.
    pub fn with_api_key_override(mut self, key: String) -> Self {
        self.api_key_override = Some(key);
        self
    }

    fn ensure_api_key(&self, spec: &AgentSpec) {
        let provider = format!("{:?}", spec.model.provider).to_lowercase();
        let env_var = api_key_env_var(&provider);

        if let Some(ref key) = self.api_key_override {
            if let Some(var) = env_var {
                unsafe { std::env::set_var(var, key) };
            }
            return;
        }

        if env_var.is_none_or(|var| std::env::var(var).is_ok()) {
            return;
        }

        if let Some(ref resolver) = self.secrets {
            if let Some(key) = resolver.resolve(&provider, &spec.id) {
                if let Some(var) = env_var {
                    unsafe { std::env::set_var(var, key) };
                }
            }
        }
    }
}

impl CoreAgentFactory for HnsxAgentFactory {
    fn create(&self, spec: &AgentSpec) -> Result<Arc<dyn Agent>> {
        self.ensure_api_key(spec);

        let sandbox_spec = spec.sandbox.clone().unwrap_or(SandboxSpec {
            policy: SandboxPolicy::None,
            runtime: SandboxRuntime::Auto,
            ..Default::default()
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
