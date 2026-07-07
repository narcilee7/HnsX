use crate::commands::run::AdapterKind;
use crate::commands::{Secrets, load_config, load_secrets, resolve_api_key};
use anyhow::{Context, Result};
use clap::Args;
use futures::StreamExt;
use hnsx_adapter::HnsxAgentFactory;
use hnsx_core::DomainLoader;
use hnsx_core::agent::InvokeContext;
use hnsx_core::agent_factory::AgentFactory;
use hnsx_core::chunk::Chunk;
use hnsx_core::Reporter;
use hnsx_core::reporter::GrpcReporter;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

#[derive(Args, Debug)]
pub struct TestArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
    /// Agent id within the domain
    #[arg(long)]
    pub agent: String,
    /// JSON input to feed the agent
    #[arg(long, default_value = "{}")]
    pub input: String,
    /// Which AgentFactory to use
    #[arg(long, value_enum, default_value_t = AdapterKind::Hnsx)]
    pub adapter: AdapterKind,
    /// Path to config file
    #[arg(long)]
    pub config: Option<PathBuf>,
    /// Path to secrets file
    #[arg(long)]
    pub secrets: Option<PathBuf>,
    /// Override the API key for the provider used by the agent
    #[arg(long)]
    pub api_key: Option<String>,
    /// Report a test invocation summary to a control plane gRPC address.
    #[arg(long)]
    pub control_plane: Option<String>,
}

struct CliSecretsResolver {
    secrets: Secrets,
}

impl hnsx_adapter::SecretResolver for CliSecretsResolver {
    fn resolve(&self, provider: &str, agent_id: &str) -> Option<String> {
        resolve_api_key(&self.secrets, provider, agent_id)
    }
}

pub fn exec(args: TestArgs) -> Result<()> {
    let rt = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
        .context("failed to build tokio runtime")?;

    rt.block_on(test_agent(args))
}

async fn test_agent(args: TestArgs) -> Result<()> {
    let _config = load_config(args.config.as_deref()).context("failed to load config")?;
    let secrets = load_secrets(args.secrets.as_deref()).context("failed to load secrets")?;

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

    let loader = DomainLoader::with_factory(factory.clone());
    let domain = loader
        .from_path(&args.domain)
        .with_context(|| format!("failed to load domain {}", args.domain))?;

    let spec = domain
        .agent_spec(&args.agent)
        .with_context(|| format!("agent '{}' not found in domain", args.agent))?;

    let input: serde_json::Value = serde_json::from_str(&args.input)
        .with_context(|| format!("invalid input JSON: {}", args.input))?;

    let agent = factory
        .create(spec)
        .with_context(|| format!("failed to create agent '{}'", args.agent))?;

    let ctx = InvokeContext {
        session_id: uuid::Uuid::new_v4().to_string(),
        domain_id: domain.spec().id.clone(),
        agent_id: args.agent.clone(),
    };

    eprintln!(
        "[hnsx test] running agent '{}' in domain '{}'",
        args.agent,
        domain.spec().id
    );

    let mut stream = agent.invoke(input, ctx.clone()).await?;
    let started_at_ms = now_ms();
    while let Some(chunk) = stream.next().await {
        match chunk {
            Chunk::Text(t) => print!("{t}"),
            Chunk::Error(e) => {
                eprintln!("\n[hnsx test error] {e}");
                std::process::exit(1);
            }
            Chunk::Artifact(_) => {}
            Chunk::Done { .. } => println!(),
        }
    }

    if let Some(addr) = args.control_plane.as_deref() {
        let reporter = GrpcReporter::connect(addr)
            .await
            .with_context(|| format!("failed to connect to control plane at {addr}"))?;
        let record = hnsx_proto::v1::InvocationRecord {
            session_id: ctx.session_id,
            domain_id: ctx.domain_id,
            started_at_ms: started_at_ms as i64,
            duration_ms: (now_ms() - started_at_ms) as i64,
            total_cost_usd: 0.0,
            prompt_tokens: 0,
            completion_tokens: 0,
        };
        reporter
            .report_invocation(&record)
            .await
            .with_context(|| "failed to report test invocation")?;
        eprintln!("[hnsx test] reported invocation to control plane at {addr}");
    }

    Ok(())
}

fn now_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as u64)
        .unwrap_or(0)
}
