use crate::commands::{Secrets, load_config, load_secrets, resolve_api_key};
use anyhow::{Context, Result};
use clap::{Args, ValueEnum};
use futures::StreamExt;
use hnsx_adapter::HnsxAgentFactory;
use hnsx_core::Artifact;
use hnsx_core::DomainLoader;
use hnsx_core::agent_factory::AgentFactory;
use hnsx_core::chunk::Chunk;
use hnsx_core::telemetry::Telemetry;
use std::io::Read;
use std::path::PathBuf;
use std::sync::Arc;

#[derive(ValueEnum, Clone, Copy, Debug, Default)]
pub enum AdapterKind {
    /// Use the HnsX factory (native adapters for HTTP providers + sandboxed
    /// CLI adapters for `provider: claude-code` / `codex`).
    #[default]
    Hnsx,
    /// Use the genai-backed factory only (OpenAI / Anthropic / Ollama /
    /// Custom). Reads API keys from env.
    Genai,
    /// Use the noop factory: every agent echoes its input. No network, no
    /// API keys needed. Useful for CI and local end-to-end smoke tests.
    Noop,
}

impl AdapterKind {
    fn as_provider_str(self) -> &'static str {
        match self {
            AdapterKind::Hnsx => "hnsx",
            AdapterKind::Genai => "genai",
            AdapterKind::Noop => "noop",
        }
    }
}

#[derive(Args, Debug)]
pub struct RunArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
    /// JSON trigger payload. Use `-` to read from stdin (defaults to `{}`)
    #[arg(long, default_value = "{}")]
    pub trigger: String,
    /// Which AgentFactory to use
    #[arg(long, value_enum, default_value_t = AdapterKind::Hnsx)]
    pub adapter: AdapterKind,
    /// Directory for JSONL traces (overrides config / env)
    #[arg(long)]
    pub trace_dir: Option<PathBuf>,
    /// Report traces and invocation summaries to a control plane gRPC address.
    #[arg(long)]
    pub control_plane: Option<String>,
    /// Path to config file
    #[arg(long)]
    pub config: Option<PathBuf>,
    /// Path to secrets file
    #[arg(long)]
    pub secrets: Option<PathBuf>,
    /// Output format
    #[arg(long, value_enum, default_value_t = OutputFormat::Text)]
    pub output: OutputFormat,
    /// Override the API key for the provider used by the entry agent
    #[arg(long)]
    pub api_key: Option<String>,
}

#[derive(ValueEnum, Clone, Copy, Debug, Default)]
pub enum OutputFormat {
    /// Print text chunks as they arrive; exit on error chunks.
    #[default]
    Text,
    /// Print a single JSON object at the end.
    Json,
    /// Print one JSON object per line (one per chunk).
    Jsonl,
}

pub fn exec(args: RunArgs) -> Result<()> {
    let rt = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
        .context("failed to build tokio runtime")?;

    rt.block_on(run(args))
}
struct CliSecretsResolver {
    secrets: Secrets,
}

impl hnsx_adapter::SecretResolver for CliSecretsResolver {
    fn resolve(&self, provider: &str, agent_id: &str) -> Option<String> {
        resolve_api_key(&self.secrets, provider, agent_id)
    }
}

async fn run(args: RunArgs) -> Result<()> {
    let config = load_config(args.config.as_deref()).context("failed to load config")?;
    let secrets = load_secrets(args.secrets.as_deref()).context("failed to load secrets")?;

    let trace_dir = args
        .trace_dir
        .clone()
        .or_else(|| config.trace_dir.clone())
        .or_else(|| std::env::var("HNSX_TRACE_DIR").ok().map(PathBuf::from));

    let factory: Arc<dyn AgentFactory> = match args.adapter {
        AdapterKind::Hnsx => {
            let mut factory = HnsxAgentFactory::new();
            if let Some(key) = args.api_key.clone() {
                factory = factory.with_api_key_override(key);
            }
            let resolver = Arc::new(CliSecretsResolver { secrets });
            Arc::new(factory.with_secret_resolver(resolver))
        }
        AdapterKind::Genai => Arc::new(hnsx_adapter::GenaiAgentFactory::new()),
        AdapterKind::Noop => Arc::new(hnsx_core::NoopFactory),
    };

    let telemetry = if let Some(dir) = trace_dir {
        Telemetry::with_dir(dir).context("failed to initialize telemetry")?
    } else {
        Telemetry::new()
            .context("failed to initialize telemetry (set HNSX_TRACE_DIR to override)")?
    };
    if let Some(addr) = args.control_plane.as_deref() {
        let reporter = hnsx_core::reporter::GrpcReporter::connect(addr)
            .await
            .with_context(|| format!("failed to connect to control plane at {addr}"))?;
        telemetry.set_reporter(Arc::new(reporter));
        eprintln!("[hnsx] reporting telemetry to control plane at {addr}");
    }
    eprintln!(
        "[hnsx] tracing per-step events to {}",
        telemetry.trace_dir().display()
    );

    let domain = DomainLoader::with_factory(factory)
        .with_telemetry(Arc::new(telemetry))
        .from_path(&args.domain)
        .with_context(|| format!("failed to load domain {}", args.domain))?;

    let trigger_json = if args.trigger == "-" {
        let mut input = String::new();
        std::io::stdin()
            .read_to_string(&mut input)
            .context("failed to read trigger from stdin")?;
        input
    } else {
        args.trigger.clone()
    };

    let trigger: serde_json::Value = serde_json::from_str(&trigger_json)
        .with_context(|| format!("invalid trigger JSON: {}", trigger_json))?;

    let mut stream = domain
        .invoke(trigger)
        .await
        .context("domain invocation failed")?;

    match args.output {
        OutputFormat::Text => output_text(&mut stream).await,
        OutputFormat::Json => output_json(&mut stream).await,
        OutputFormat::Jsonl => output_jsonl(&mut stream).await,
    }
}

async fn output_text(stream: &mut futures::stream::BoxStream<'static, Chunk>) -> Result<()> {
    while let Some(chunk) = stream.next().await {
        match chunk {
            Chunk::Text(t) => print!("{t}"),
            Chunk::Error(e) => {
                eprintln!("\n[hnsx error] {e}");
                std::process::exit(1);
            }
            Chunk::Artifact(_) => {}
            Chunk::Done { .. } => println!(),
        }
    }
    Ok(())
}

async fn output_json(stream: &mut futures::stream::BoxStream<'static, Chunk>) -> Result<()> {
    let mut text = String::new();
    let mut variables = serde_json::Value::Null;
    let mut prompt_tokens: u64 = 0;
    let mut completion_tokens: u64 = 0;
    let mut cost_usd: f64 = 0.0;

    while let Some(chunk) = stream.next().await {
        match chunk {
            Chunk::Text(t) => text.push_str(&t),
            Chunk::Error(e) => {
                let err = serde_json::json!({"error": e});
                println!("{}", serde_json::to_string_pretty(&err).unwrap());
                std::process::exit(1);
            }
            Chunk::Artifact(Artifact::TokenUsage {
                prompt,
                completion,
                cost_usd: cost,
            }) => {
                prompt_tokens += prompt;
                completion_tokens += completion;
                cost_usd += cost;
            }
            Chunk::Done { variables: vars } => variables = vars,
            _ => {}
        }
    }

    let out = serde_json::json!({
        "output": text,
        "variables": variables,
        "usage": {
            "prompt_tokens": prompt_tokens,
            "completion_tokens": completion_tokens,
            "cost_usd": cost_usd,
        }
    });
    println!("{}", serde_json::to_string_pretty(&out).unwrap());
    Ok(())
}

async fn output_jsonl(stream: &mut futures::stream::BoxStream<'static, Chunk>) -> Result<()> {
    while let Some(chunk) = stream.next().await {
        match chunk {
            Chunk::Error(e) => {
                let line = serde_json::json!({"type": "error", "message": e});
                println!("{}", serde_json::to_string(&line).unwrap());
                std::process::exit(1);
            }
            other => {
                let line = match other {
                    Chunk::Text(t) => serde_json::json!({"type": "text", "content": t}),
                    Chunk::Artifact(Artifact::TokenUsage {
                        prompt,
                        completion,
                        cost_usd,
                    }) => serde_json::json!({
                        "type": "usage",
                        "prompt_tokens": prompt,
                        "completion_tokens": completion,
                        "cost_usd": cost_usd,
                    }),
                    Chunk::Done { variables } => {
                        serde_json::json!({"type": "done", "variables": variables})
                    }
                    Chunk::Artifact(Artifact::FileChanges(changes)) => {
                        serde_json::json!({"type": "file_changes", "changes": changes})
                    }
                    Chunk::Error(_) => unreachable!(),
                };
                println!("{}", serde_json::to_string(&line).unwrap());
            }
        }
    }
    Ok(())
}
