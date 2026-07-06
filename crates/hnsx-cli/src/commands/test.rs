use anyhow::Result;
use clap::Args;

#[derive(Args, Debug)]
pub struct TestArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
    /// Agent id within the domain
    #[arg(long)]
    pub agent: String,
    /// JSON input to feed the agent
    #[arg(long)]
    pub input: String,
}

pub fn exec(args: TestArgs) -> Result<()> {
    println!(
        "hnsx test is not yet implemented (domain={}, agent={})",
        args.domain, args.agent
    );
    Ok(())
}
