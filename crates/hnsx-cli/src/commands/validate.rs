use anyhow::Result;
use clap::Args;

use hnsx_core::DomainSpec;

#[derive(Args, Debug)]
pub struct ValidateArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
}

pub fn exec(args: ValidateArgs) -> Result<()> {
    let text = std::fs::read_to_string(&args.domain)?;
    let spec: DomainSpec = serde_yaml::from_str(&text)?;
    println!(
        "validated domain id={} version={} agents={} steps={}",
        spec.id,
        spec.version,
        spec.agents.len(),
        spec.workflow.steps.len(),
    );
    Ok(())
}
