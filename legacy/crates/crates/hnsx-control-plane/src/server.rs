//! gRPC server wiring for the control plane.
//!
//! `ControlPlaneServer` builds a `Router` exposing Registry, Scheduler,
//! Discovery and Telemetry services over a shared `SqliteStore`. It also
//! exposes a Prometheus `/metrics` endpoint and a REST API for the Web UI.

use std::{future::Future, net::SocketAddr, path::PathBuf};

use axum::{Router, routing::get};
use futures::future::TryFutureExt;
use metrics_exporter_prometheus::PrometheusHandle;
use tokio::net::TcpListener;
use tokio_stream::Stream;
use tonic::transport::Server;
use tower_http::cors::{Any, CorsLayer};
use tower_http::services::{ServeDir, ServeFile};

use crate::{
    discovery::DiscoveryService,
    http_api,
    proto::{
        discovery_server::DiscoveryServer, registry_server::RegistryServer,
        scheduler_server::SchedulerServer, telemetry_server::TelemetryServer,
    },
    registry::RegistryService,
    scheduler::SchedulerService,
    store::SqliteStore,
    telemetry::TelemetryService,
};

/// Control-plane server: gRPC + Prometheus metrics + REST API + Web UI.
#[derive(Clone)]
pub struct ControlPlaneServer {
    store: SqliteStore,
    heartbeat_timeout_ms: i64,
    metrics_handle: Option<PrometheusHandle>,
    static_dir: Option<PathBuf>,
}

impl ControlPlaneServer {
    /// Create a new server using the provided SQLite store.
    pub fn new(store: SqliteStore) -> Self {
        Self {
            store,
            heartbeat_timeout_ms: 60_000,
            metrics_handle: None,
            static_dir: None,
        }
    }

    /// Attach a Prometheus metrics handle so that `/metrics` can be served.
    #[must_use]
    pub fn with_metrics_handle(mut self, handle: PrometheusHandle) -> Self {
        self.metrics_handle = Some(handle);
        self
    }

    /// Serve the built Web UI from `dir` (e.g. `web/dist`).
    #[must_use]
    pub fn with_static_dir(mut self, dir: impl Into<PathBuf>) -> Self {
        self.static_dir = Some(dir.into());
        self
    }

    /// Set the instance heartbeat timeout in milliseconds.
    #[must_use]
    pub fn with_heartbeat_timeout_ms(mut self, ms: i64) -> Self {
        self.heartbeat_timeout_ms = ms;
        self
    }

    fn services(
        &self,
    ) -> (
        RegistryServer<RegistryService>,
        SchedulerServer<SchedulerService>,
        DiscoveryServer<DiscoveryService>,
        TelemetryServer<TelemetryService>,
    ) {
        (
            RegistryServer::new(RegistryService::new(self.store.clone())),
            SchedulerServer::new(SchedulerService::new(
                self.store.clone(),
                self.heartbeat_timeout_ms,
            )),
            DiscoveryServer::new(DiscoveryService::new(self.store.clone())),
            TelemetryServer::new(TelemetryService::new(self.store.clone())),
        )
    }

    /// Build the axum router for `/metrics`, the REST API and the Web UI.
    fn http_router(&self) -> Router {
        let cors = CorsLayer::new()
            .allow_origin(Any)
            .allow_methods(Any)
            .allow_headers(Any);
        let mut router = http_api::router(self.store.clone()).layer(cors);
        if let Some(handle) = self.metrics_handle.clone() {
            router = router.route("/metrics", get(move || async move { handle.render() }));
        }
        if let Some(dir) = self.static_dir.as_ref() {
            let index = dir.join("index.html");
            router = router.nest_service("/", ServeDir::new(dir).fallback(ServeFile::new(index)));
        }
        router
    }

    /// Serve both gRPC and HTTP on the same `addr` until the process is interrupted.
    ///
    /// HTTP is served on `addr.port() + 1`. If `addr` has port `0`, the OS picks
    /// the gRPC port and the HTTP port is computed from the bound address.
    ///
    /// # Errors
    ///
    /// Returns an error if either server cannot bind or run.
    pub async fn serve(&self, addr: SocketAddr) -> anyhow::Result<()> {
        let grpc_listener = TcpListener::bind(addr).await?;
        let grpc_addr = grpc_listener.local_addr()?;
        let http_addr = SocketAddr::new(grpc_addr.ip(), grpc_addr.port() + 1);
        let incoming = tokio_stream::wrappers::TcpListenerStream::new(grpc_listener);

        let (registry, scheduler, discovery, telemetry) = self.services();
        let grpc = Server::builder()
            .add_service(registry)
            .add_service(scheduler)
            .add_service(discovery)
            .add_service(telemetry)
            .serve_with_incoming_shutdown(incoming, std::future::pending());

        let http = async {
            let listener = TcpListener::bind(http_addr).await?;
            let router = self.http_router();
            axum::serve(listener, router)
                .await
                .map_err(anyhow::Error::from)
        };

        tokio::try_join!(grpc.map_err(anyhow::Error::from), http)?;
        Ok(())
    }

    /// Serve gRPC over a bound listener and HTTP over a bound listener.
    ///
    /// Both listeners are assumed to already be bound by the caller, which
    /// avoids port races in tests.
    ///
    /// # Errors
    ///
    /// Returns an error if either server cannot run.
    pub async fn serve_with_both_bound(
        &self,
        grpc_listener: TcpListener,
        http_listener: TcpListener,
    ) -> anyhow::Result<()> {
        let incoming = tokio_stream::wrappers::TcpListenerStream::new(grpc_listener);

        let (registry, scheduler, discovery, telemetry) = self.services();
        let grpc = Server::builder()
            .add_service(registry)
            .add_service(scheduler)
            .add_service(discovery)
            .add_service(telemetry)
            .serve_with_incoming_shutdown(incoming, std::future::pending());

        let http = async {
            let router = self.http_router();
            axum::serve(http_listener, router)
                .await
                .map_err(anyhow::Error::from)
        };

        tokio::try_join!(grpc.map_err(anyhow::Error::from), http)?;
        Ok(())
    }

    /// Serve gRPC over a bound listener and HTTP on a separate address.
    ///
    /// This is useful when the caller needs to know the bound port (e.g. port
    /// `0`) before printing startup messages.
    ///
    /// # Errors
    ///
    /// Returns an error if either server cannot bind or run.
    pub async fn serve_with_bound(
        &self,
        grpc_listener: TcpListener,
        http_addr: SocketAddr,
    ) -> anyhow::Result<()> {
        let http_listener = TcpListener::bind(http_addr).await?;
        self.serve_with_both_bound(grpc_listener, http_listener)
            .await
    }

    /// Serve gRPC over an existing incoming stream until `shutdown` resolves.
    /// HTTP is not started by this variant; use [`serve`] for full endpoints.
    ///
    /// This is useful in tests where the listener is bound to port `0`.
    ///
    /// # Errors
    ///
    /// Returns an error if the tonic server cannot run.
    pub async fn serve_with_incoming<I, IO, IE, SF>(
        &self,
        incoming: I,
        shutdown: SF,
    ) -> anyhow::Result<()>
    where
        I: Stream<Item = Result<IO, IE>> + Send + 'static,
        IO: tokio::io::AsyncRead
            + tokio::io::AsyncWrite
            + tonic::transport::server::Connected
            + Send
            + Unpin
            + 'static,
        IO::ConnectInfo: Clone + Send + 'static,
        IE: Into<std::io::Error> + std::error::Error + Sync + Send + 'static,
        SF: Future<Output = ()> + Send + 'static,
    {
        let (registry, scheduler, discovery, telemetry) = self.services();
        Ok(Server::builder()
            .add_service(registry)
            .add_service(scheduler)
            .add_service(discovery)
            .add_service(telemetry)
            .serve_with_incoming_shutdown(incoming, shutdown)
            .await?)
    }
}
