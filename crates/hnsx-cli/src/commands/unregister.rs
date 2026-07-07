use anyhow::{Context, Result};
use clap::Args;
use hnsx_proto::v1::{DomainRef, registry_client::RegistryClient};
use tonic::transport::Channel;

#[derive(Args, Debug)]
pub struct UnregisterArgs {
    /// Domain id to unregister.
    #[arg(long)]
    pub id: String,
    /// Domain version to unregister.
    #[arg(long)]
    pub version: String,
    /// Control plane gRPC address.
    #[arg(long, default_value = "http://127.0.0.1:50051")]
    pub control_plane: String,
}

pub fn exec(args: UnregisterArgs) -> Result<()> {
    let rt = tokio::runtime::Runtime::new().context("create tokio runtime")?;
    rt.block_on(run(args))
}

async fn run(args: UnregisterArgs) -> Result<()> {
    let mut client = RegistryClient::<Channel>::connect(args.control_plane.clone())
        .await
        .with_context(|| format!("failed to connect to control plane at {}", args.control_plane))?;

    let req = DomainRef {
        id: args.id.clone(),
        version: args.version.clone(),
    };

    client
        .unregister_domain(req)
        .await
        .with_context(|| format!("failed to unregister domain at {}", args.control_plane))?;

    println!(
        "unregistered domain {}@{} from {}",
        args.id, args.version, args.control_plane
    );
    Ok(())
}
