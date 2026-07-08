use std::net::SocketAddr;
use std::sync::Arc;

use anyhow::{Context, Result};
use clap::Args;
use hnsx_adapter::HnsxAgentFactory;
use hnsx_core::DomainLoader;
use hnsx_core::package::unpack_domain;
use hnsx_core::telemetry::Telemetry;

use crate::runtime::DomainRuntime;

#[derive(Args, Debug)]
pub struct DevArgs {
    /// Path to the domain YAML or packaged artifact (.hnsx.tar)
    #[arg(long)]
    pub domain: String,
    /// gRPC address to bind for the runtime server.
    #[arg(long, default_value = "127.0.0.1:0")]
    pub bind: String,
    /// Control plane gRPC address.
    #[arg(long, default_value = "http://127.0.0.1:50051")]
    pub control_plane: String,
}

pub fn exec(args: DevArgs) -> Result<()> {
    let rt = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
        .context("failed to build tokio runtime")?;
    rt.block_on(run(args))
}

async fn run(args: DevArgs) -> Result<()> {
    let (domain_yaml, _temp_dir) = if args.domain.ends_with(".hnsx.tar") {
        let temp = tempfile::tempdir().context("create temp dir")?;
        let (_manifest, yaml) = unpack_domain(&args.domain, temp.path())
            .with_context(|| format!("unpack {}", args.domain))?;
        (yaml, Some(temp))
    } else {
        (std::path::PathBuf::from(&args.domain), None)
    };

    let factory: Arc<dyn hnsx_core::agent_factory::AgentFactory> =
        Arc::new(HnsxAgentFactory::new());

    let telemetry = Telemetry::new()
        .context("failed to initialize telemetry (set HNSX_TRACE_DIR to override)")?;
    let domain = DomainLoader::with_factory(factory)
        .with_telemetry(Arc::new(telemetry))
        .from_path(&domain_yaml)
        .with_context(|| format!("failed to load domain {}", domain_yaml.display()))?;

    let _domain_id = domain.spec().id.clone();
    let hostname = hostname::get()
        .unwrap_or_else(|_| std::ffi::OsString::from("unknown"))
        .to_string_lossy()
        .to_string();
    let instance_id = format!("{}-{}", hostname, uuid::Uuid::new_v4());

    let bind_addr: SocketAddr = args
        .bind
        .parse()
        .with_context(|| format!("invalid bind address: {}", args.bind))?;

    let runtime = DomainRuntime::new(domain, instance_id, args.control_plane);
    let (tx, rx) = tokio::sync::oneshot::channel();

    tokio::select! {
        result = runtime.serve(bind_addr, rx) => result,
        _ = tokio::signal::ctrl_c() => {
            let _ = tx.send(());
            Ok(())
        }
    }
}
