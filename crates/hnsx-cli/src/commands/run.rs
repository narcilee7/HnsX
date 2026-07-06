use anyhow::Result;
use clap::Args;

#[derive(Args, Debug)]
pub struct RunArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
    /// JSON trigger payload
    #[arg(long)]
    pub trigger: String,
}

pub fn exec(args: RunArgs) -> Result<()> {
    println!(
        "hnsx run is not yet implemented (domain={}, trigger={})",
        args.domain, args.trigger
    );
    Ok(())
}
