//! gRPC server wiring for the control plane.
//!
//! `ControlPlaneServer` builds a `Router` exposing Registry, Scheduler,
//! Discovery and Telemetry services over a shared `SqliteStore`.

use std::{future::Future, net::SocketAddr};

use tokio_stream::Stream;
use tonic::transport::Server;

use crate::{
    discovery::DiscoveryService,
    proto::{
        discovery_server::DiscoveryServer,
        registry_server::RegistryServer,
        scheduler_server::SchedulerServer,
        telemetry_server::TelemetryServer,
    },
    registry::RegistryService,
    scheduler::SchedulerService,
    store::SqliteStore,
    telemetry::TelemetryService,
};

/// Control-plane gRPC server.
#[derive(Clone)]
pub struct ControlPlaneServer {
    store: SqliteStore,
    heartbeat_timeout_ms: i64,
}

impl ControlPlaneServer {
    /// Create a new server using the provided SQLite store.
    pub fn new(store: SqliteStore) -> Self {
        Self {
            store,
            heartbeat_timeout_ms: 60_000,
        }
    }

    /// Set the instance heartbeat timeout in milliseconds.
    #[must_use]
    pub fn with_heartbeat_timeout_ms(mut self, ms: i64) -> Self {
        self.heartbeat_timeout_ms = ms;
        self
    }

    fn services(&self,
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

    /// Serve the gRPC control plane on `addr` until the process is interrupted.
    ///
    /// # Errors
    ///
    /// Returns an error if the tonic server cannot bind or run.
    pub async fn serve(&self,
        addr: SocketAddr,
    ) -> anyhow::Result<()> {
        let (registry, scheduler, discovery, telemetry) = self.services();
        Ok(Server::builder()
            .add_service(registry)
            .add_service(scheduler)
            .add_service(discovery)
            .add_service(telemetry)
            .serve(addr)
            .await?)
    }

    /// Serve over an existing incoming stream until `shutdown` resolves.
    ///
    /// This is useful in tests where the listener is bound to port `0` and the
    /// actual local address is needed to connect clients.
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
