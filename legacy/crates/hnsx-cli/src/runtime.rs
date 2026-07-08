//! Long-running domain runtime used by `hnsx dev` and `hnsx deploy`.
//!
//! A runtime loads a domain, registers itself as an agent instance with the
//! control plane, sends periodic heartbeats, and serves the `Runtime` gRPC
//! service so clients can trigger workflow invocations.

use std::net::SocketAddr;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use futures::StreamExt;
use hnsx_core::chunk::Chunk;
use hnsx_core::domain::Domain;
use hnsx_proto::v1::{
    InstanceInfo, InstanceRef, TriggerRequest, TriggerResponse,
    runtime_server::{Runtime, RuntimeServer},
    scheduler_client::SchedulerClient,
};
use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use tonic::transport::Channel;
use tonic::{Request, Response, Status};

/// Long-running domain runtime.
pub struct DomainRuntime {
    domain: Arc<dyn Domain>,
    instance_id: String,
    control_plane: String,
    tags: Vec<String>,
    region: String,
}

impl DomainRuntime {
    /// Create a runtime for the given domain.
    pub fn new(domain: Arc<dyn Domain>, instance_id: String, control_plane: String) -> Self {
        Self {
            domain,
            instance_id,
            control_plane,
            tags: vec!["runtime".into()],
            region: std::env::var("HNSX_REGION").unwrap_or_else(|_| "local".into()),
        }
    }

    /// Start registration, heartbeats, and the gRPC server. Blocks until
    /// shutdown is received.
    pub async fn serve(
        self,
        addr: SocketAddr,
        shutdown: tokio::sync::oneshot::Receiver<()>,
    ) -> Result<()> {
        let mut scheduler = SchedulerClient::<Channel>::connect(self.control_plane.clone())
            .await
            .context("connect to control plane scheduler")?;

        let domain_id = self.domain.spec().id.clone();
        let instance_id = self.instance_id.clone();

        // Register instance.
        scheduler
            .register_instance(InstanceInfo {
                instance_id: instance_id.clone(),
                domain_id: domain_id.clone(),
                tags: self.tags.clone(),
                region: self.region.clone(),
                capabilities: vec!["trigger".into()],
            })
            .await
            .context("register instance")?;

        // Heartbeat loop.
        let heartbeat_instance_id = instance_id.clone();
        let mut heartbeat_scheduler = scheduler.clone();
        let heartbeat_handle = tokio::spawn(async move {
            let mut interval = tokio::time::interval(Duration::from_secs(30));
            loop {
                interval.tick().await;
                if let Err(e) = heartbeat_scheduler
                    .heartbeat(InstanceRef {
                        instance_id: heartbeat_instance_id.clone(),
                    })
                    .await
                {
                    eprintln!("[runtime] heartbeat failed: {e}");
                }
            }
        });

        // gRPC server.
        let service = RuntimeService {
            domain: self.domain.clone(),
            instance_id: instance_id.clone(),
        };
        let server = tonic::transport::Server::builder()
            .add_service(RuntimeServer::new(service))
            .serve_with_shutdown(addr, async {
                let _ = shutdown.await;
            });

        println!(
            "[runtime] {} listening on {} for domain {}",
            instance_id, addr, domain_id
        );
        let result = server.await.context("runtime server");

        heartbeat_handle.abort();

        // Best-effort unregister.
        let _ = scheduler
            .unregister_instance(InstanceRef { instance_id })
            .await;

        result
    }
}

struct RuntimeService {
    domain: Arc<dyn Domain>,
    instance_id: String,
}

#[tonic::async_trait]
impl Runtime for RuntimeService {
    type TriggerStream = ReceiverStream<Result<TriggerResponse, Status>>;

    async fn trigger(
        &self,
        request: Request<TriggerRequest>,
    ) -> Result<Response<Self::TriggerStream>, Status> {
        let req = request.into_inner();
        let domain_id = self.domain.spec().id.clone();
        if req.domain_id != domain_id {
            return Err(Status::invalid_argument(format!(
                "runtime serves domain '{}' but request was for '{}'",
                domain_id, req.domain_id
            )));
        }

        let trigger: serde_json::Value = serde_json::from_str(&req.payload)
            .map_err(|e| Status::invalid_argument(format!("invalid trigger JSON: {e}")))?;

        let mut stream = self
            .domain
            .invoke(trigger)
            .await
            .map_err(|e| Status::internal(format!("invoke failed: {e}")))?;

        let (tx, rx) = mpsc::channel::<Result<TriggerResponse, Status>>(128);
        tokio::spawn(async move {
            while let Some(chunk) = stream.next().await {
                let resp = match chunk {
                    Chunk::Text(t) => TriggerResponse {
                        item: Some(hnsx_proto::v1::trigger_response::Item::Text(
                            hnsx_proto::v1::ChunkText { content: t },
                        )),
                    },
                    Chunk::Error(e) => TriggerResponse {
                        item: Some(hnsx_proto::v1::trigger_response::Item::Error(
                            hnsx_proto::v1::ChunkError { message: e },
                        )),
                    },
                    Chunk::Artifact(_) => continue,
                    Chunk::Done { variables } => TriggerResponse {
                        item: Some(hnsx_proto::v1::trigger_response::Item::Done(
                            hnsx_proto::v1::ChunkDone {
                                variables: variables.to_string(),
                            },
                        )),
                    },
                };
                if tx.send(Ok(resp)).await.is_err() {
                    break;
                }
            }
        });

        Ok(Response::new(ReceiverStream::new(rx)))
    }
}
