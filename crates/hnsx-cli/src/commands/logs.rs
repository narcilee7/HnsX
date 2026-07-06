use anyhow::Result;
use clap::Args;

#[derive(Args, Debug)]
pub struct LogsArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
    /// Follow log output
    #[arg(long, short = 'f', default_value_t = false)]
    pub follow: bool,
}

pub fn exec(args: LogsArgs) -> Result<()> {
    println!(
        "hnsx logs is not yet implemented (domain={}, follow={})",
        args.domain, args.follow
    );
    Ok(())
}
