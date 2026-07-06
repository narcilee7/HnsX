use std::sync::Arc;

use anyhow::{Context, Result};
use clap::{Args, ValueEnum};
use futures::StreamExt;

use hnsx_adapter::HnsxAgentFactory;
use hnsx_core::DomainLoader;
use hnsx_core::agent_factory::AgentFactory;
use hnsx_core::chunk::Chunk;
use hnsx_core::telemetry::Telemetry;

#[derive(ValueEnum, Clone, Copy, Debug, Default)]
pub enum AdapterKind {
    /// Use the HnsX factory (genai for HTTP providers + sandboxed Claude Code
    /// CLI for `provider: claude-code`).
    #[default]
    Hnsx,
    /// Use the genai-backed factory only (OpenAI / Anthropic / Ollama /
    /// Custom). Reads API keys from env.
    Genai,
    /// Use the noop factory: every agent echoes its input. No network, no
    /// API keys needed. Useful for CI and local end-to-end smoke tests.
    Noop,
}

#[derive(Args, Debug)]
pub struct RunArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
    /// JSON trigger payload (defaults to `{}`)
    #[arg(long, default_value = "{}")]
    pub trigger: String,
    /// Which AgentFactory to use. Default: `hnsx`.
    #[arg(long, value_enum, default_value_t = AdapterKind::Hnsx)]
    pub adapter: AdapterKind,
}

pub fn exec(args: RunArgs) -> Result<()> {
    let rt = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
        .context("failed to build tokio runtime")?;

    rt.block_on(run(args))
}

async fn run(args: RunArgs) -> Result<()> {
    let factory: Arc<dyn AgentFactory> = match args.adapter {
        AdapterKind::Hnsx => Arc::new(HnsxAgentFactory::new()),
        AdapterKind::Genai => Arc::new(hnsx_adapter::GenaiAgentFactory::new()),
        AdapterKind::Noop => Arc::new(hnsx_core::NoopFactory),
    };

    let telemetry = Telemetry::new()
        .context("failed to initialize telemetry (set HNSX_TRACE_DIR to override)")?;
    eprintln!(
        "[hnsx] tracing per-step events to {}",
        telemetry.trace_dir().display()
    );

    let domain = DomainLoader::with_factory(factory)
        .with_telemetry(Arc::new(telemetry))
        .from_path(&args.domain)
        .with_context(|| format!("failed to load domain {}", args.domain))?;

    let trigger: serde_json::Value = serde_json::from_str(&args.trigger)
        .with_context(|| format!("invalid trigger JSON: {}", args.trigger))?;

    let mut stream = domain
        .invoke(trigger)
        .await
        .context("domain invocation failed")?;

    while let Some(chunk) = stream.next().await {
        match chunk {
            Chunk::Text(t) => print!("{t}"),
            Chunk::Error(e) => {
                eprintln!("\n[hnsx error] {e}");
                std::process::exit(1);
            }
            Chunk::Artifact(_) => {
                // Skip artifacts in the default text output. `hnsx logs`
                // / `hnsx metrics` will surface them in later phases.
            }
            Chunk::Done { .. } => {
                println!();
            }
        }
    }
    Ok(())
}
