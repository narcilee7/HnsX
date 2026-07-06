use std::path::PathBuf;

use anyhow::Result;
use clap::Args;

use crate::deploy::docker;

#[derive(Args, Debug)]
pub struct DeployArgs {
    /// Path to the packaged domain artifact (.hnsx.tar)
    #[arg(long)]
    pub artifact: String,
    /// Deployment target (docker)
    #[arg(long, default_value = "docker")]
    pub target: String,
    /// Control plane gRPC address.
    #[arg(long, default_value = "http://127.0.0.1:50051")]
    pub control_plane: String,
    /// Container name (default: random)
    #[arg(long)]
    pub name: Option<String>,
    /// Host port to expose the runtime gRPC server (default: none)
    #[arg(long)]
    pub port: Option<u16>,
}

pub fn exec(args: DeployArgs) -> Result<()> {
    let artifact = PathBuf::from(&args.artifact);
    match args.target.as_str() {
        "docker" => docker::deploy(
            &artifact,
            &args.control_plane,
            args.name.as_deref(),
            args.port,
        ),
        other => Err(anyhow::anyhow!("unsupported deploy target: {other}")),
    }
}
