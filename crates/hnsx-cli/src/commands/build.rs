use anyhow::{Context, Result};
use clap::Args;

use hnsx_core::package::pack_domain;

#[derive(Args, Debug)]
pub struct BuildArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
    /// Output artifact path (default: <id>-<version>.hnsx.tar in the current directory)
    #[arg(long)]
    pub output: Option<String>,
}

pub fn exec(args: BuildArgs) -> Result<()> {
    let output = match args.output {
        Some(o) => o,
        None => {
            // Derive default output name from the domain spec.
            let yaml = std::fs::read_to_string(&args.domain)
                .with_context(|| format!("read {}", args.domain))?;
            let spec: hnsx_core::DomainSpec = serde_yaml::from_str(&yaml)
                .with_context(|| format!("parse {}", args.domain))?;
            format!("{}-{}.hnsx.tar", spec.id, spec.version)
        }
    };

    pack_domain(&args.domain, &output).with_context(|| {
        format!(
            "failed to pack domain {} into {}",
            args.domain, output
        )
    })?;

    println!("Built {}", output);
    Ok(())
}
