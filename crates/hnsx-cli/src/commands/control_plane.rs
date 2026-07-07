use std::net::SocketAddr;

use anyhow::{Context, Result};
use clap::Args;
use hnsx_control_plane::metrics;
use hnsx_control_plane::server::ControlPlaneServer;
use hnsx_control_plane::store::SqliteStore;

use tokio::net::TcpListener;

#[derive(Args, Debug)]
pub struct ControlPlaneArgs {
    /// gRPC + HTTP bind address.
    #[arg(long, default_value = "127.0.0.1:50051")]
    pub addr: String,
    /// SQLite database path.
    #[arg(long, default_value = "hnsx.db")]
    pub db: String,
    /// Directory with the built Web UI (optional).
    #[arg(long)]
    pub static_dir: Option<String>,
}

pub fn exec(args: ControlPlaneArgs) -> Result<()> {
    let rt = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
        .context("failed to build tokio runtime")?;
    rt.block_on(run(args))
}

async fn run(args: ControlPlaneArgs) -> Result<()> {
    let addr: SocketAddr = args
        .addr
        .parse()
        .with_context(|| format!("invalid address: {}", args.addr))?;

    let grpc_listener = TcpListener::bind(addr)
        .await
        .with_context(|| format!("failed to bind gRPC listener on {}", addr))?;
    let grpc_addr = grpc_listener.local_addr()?;
    let http_addr = SocketAddr::new(grpc_addr.ip(), grpc_addr.port() + 1);

    let store = SqliteStore::open(&args.db)
        .await
        .with_context(|| format!("open SQLite store at {}", args.db))?;

    let metrics_handle = metrics::install().context("install prometheus metrics")?;

    let mut server = ControlPlaneServer::new(store).with_metrics_handle(metrics_handle);
    if let Some(dir) = args.static_dir {
        server = server.with_static_dir(dir);
    }

    println!(
        "[control-plane] serving gRPC on {} and HTTP on {}",
        grpc_addr, http_addr
    );
    server
        .serve_with_bound(grpc_listener, http_addr)
        .await
        .context("serve control plane")
}
