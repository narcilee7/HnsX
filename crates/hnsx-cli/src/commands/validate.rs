use anyhow::Result;
use clap::Args;

use hnsx_core::DomainLoader;

#[derive(Args, Debug)]
pub struct ValidateArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
}

pub fn exec(args: ValidateArgs) -> Result<()> {
    // Goes through DomainLoader so it picks up structural validation
    // (id uniqueness, workflow.entry, step agent references) in addition to
    // raw YAML parsing.
    let domain = DomainLoader::new().from_path(&args.domain)?;
    let spec = domain.spec();
    println!(
        "validated domain id={} version={} agents={} steps={}",
        spec.id,
        spec.version,
        spec.agents.len(),
        spec.workflow.steps.len(),
    );
    Ok(())
}
