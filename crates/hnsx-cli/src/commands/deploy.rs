use anyhow::Result;
use clap::Args;

#[derive(Args, Debug)]
pub struct DeployArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
    /// Deployment target (k8s | docker | lambda)
    #[arg(long, default_value = "docker")]
    pub target: String,
    /// Kubernetes namespace (only when target=k8s)
    #[arg(long)]
    pub namespace: Option<String>,
}

pub fn exec(args: DeployArgs) -> Result<()> {
    println!(
        "hnsx deploy is not yet implemented (domain={}, target={})",
        args.domain, args.target
    );
    Ok(())
}
