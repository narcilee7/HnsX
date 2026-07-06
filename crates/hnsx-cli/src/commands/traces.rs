use anyhow::Result;
use clap::Args;

#[derive(Args, Debug)]
pub struct TracesArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
    /// Look back this duration (e.g. "1h", "30m")
    #[arg(long, default_value = "1h")]
    pub since: String,
}

pub fn exec(args: TracesArgs) -> Result<()> {
    println!(
        "hnsx traces is not yet implemented (domain={}, since={})",
        args.domain, args.since
    );
    Ok(())
}
