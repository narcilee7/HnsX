use anyhow::Result;
use clap::Args;

#[derive(Args, Debug)]
pub struct MetricsArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
}

pub fn exec(args: MetricsArgs) -> Result<()> {
    println!(
        "hnsx metrics is not yet implemented (domain={})",
        args.domain
    );
    Ok(())
}
