use anyhow::Result;
use clap::Args;

#[derive(Args, Debug)]
pub struct DevArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
}

pub fn exec(args: DevArgs) -> Result<()> {
    println!("hnsx dev is not yet implemented (domain={})", args.domain);
    Ok(())
}
