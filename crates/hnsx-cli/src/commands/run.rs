use anyhow::{Context, Result};
use clap::Args;
use futures::StreamExt;

use hnsx_core::DomainLoader;
use hnsx_core::chunk::Chunk;

#[derive(Args, Debug)]
pub struct RunArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
    /// JSON trigger payload (defaults to `{}`)
    #[arg(long, default_value = "{}")]
    pub trigger: String,
}

pub fn exec(args: RunArgs) -> Result<()> {
    // Build a small tokio runtime. We use a single-threaded runtime
    // because the workflow engine is purely cooperative for 1.2.
    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .context("failed to build tokio runtime")?;

    rt.block_on(run(args))
}

async fn run(args: RunArgs) -> Result<()> {
    let domain = DomainLoader::new()
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
