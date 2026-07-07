use anyhow::{Context, Result};
use clap::Args;
use hnsx_proto::v1::{DomainSpec, registry_client::RegistryClient};
use tonic::transport::Channel;

#[derive(Args, Debug)]
pub struct RegisterArgs {
    /// Path to the domain YAML file to register.
    #[arg(long)]
    pub domain: String,
    /// Control plane gRPC address.
    #[arg(long, default_value = "http://127.0.0.1:50051")]
    pub control_plane: String,
}

pub fn exec(args: RegisterArgs) -> Result<()> {
    let rt = tokio::runtime::Runtime::new().context("create tokio runtime")?;
    rt.block_on(run(args))
}

async fn run(args: RegisterArgs) -> Result<()> {
    let yaml_body = std::fs::read_to_string(&args.domain)
        .with_context(|| format!("failed to read domain YAML {}", args.domain))?;

    let spec = parse_domain_spec(&yaml_body)
        .with_context(|| format!("failed to parse domain YAML {}", args.domain))?;

    let mut client = RegistryClient::<Channel>::connect(args.control_plane.clone())
        .await
        .with_context(|| {
            format!(
                "failed to connect to control plane at {}",
                args.control_plane
            )
        })?;

    let req = DomainSpec {
        id: spec.id.clone(),
        version: spec.version.clone(),
        yaml_body: yaml_body.clone(),
    };

    let resp = client
        .register_domain(req)
        .await
        .with_context(|| format!("failed to register domain at {}", args.control_plane))?
        .into_inner();

    println!(
        "registered domain {}@{} at {}",
        resp.id, resp.version, args.control_plane
    );
    Ok(())
}

fn parse_domain_spec(yaml: &str) -> Result<hnsx_core::domain::DomainSpec> {
    Ok(serde_yaml::from_str(yaml)?)
}
