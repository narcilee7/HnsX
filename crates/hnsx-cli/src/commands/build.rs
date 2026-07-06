use anyhow::Result;
use clap::Args;

#[derive(Args, Debug)]
pub struct BuildArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
    /// Output artifact path
    #[arg(long)]
    pub output: String,
}

pub fn exec(args: BuildArgs) -> Result<()> {
    println!(
        "hnsx build is not yet implemented (domain={}, output={})",
        args.domain, args.output
    );
    Ok(())
}
