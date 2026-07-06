//! Integration test for the control-plane gRPC services.
//!
//! Starts a real tonic server on a random local port and exercises Registry,
//! Scheduler, Discovery and Telemetry end-to-end.

use std::net::SocketAddr;

use hnsx_control_plane::{
    proto::{
        DiscoverRequest, DomainRef, DomainSpec, Empty, InstanceInfo, InstanceRef,
        QueryTraceRequest, TraceRecord,
        discovery_client::DiscoveryClient,
        registry_client::RegistryClient,
        scheduler_client::SchedulerClient,
        telemetry_client::TelemetryClient,
    },
    server::ControlPlaneServer,
    store::SqliteStore,
};
use tokio::net::TcpListener;
use tokio_stream::wrappers::TcpListenerStream;

async fn start_server() -> (SocketAddr, tokio::sync::oneshot::Sender<()>) {
    let store = SqliteStore::open_in_memory().await.unwrap();
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let incoming = TcpListenerStream::new(listener);

    let (shutdown_tx, shutdown_rx) = tokio::sync::oneshot::channel();
    tokio::spawn(async move {
        let server = ControlPlaneServer::new(store);
        let serve = server.serve_with_incoming(
            incoming,
            async {
                let _ = shutdown_rx.await;
            },
        );
        let _ = serve.await;
    });

    (addr, shutdown_tx)
}

#[tokio::test]
async fn registry_round_trip() {
    let (addr, _shutdown) = start_server().await;
    let mut client = RegistryClient::connect(format!("http://{addr}")).await.unwrap();

    client
        .register_domain(DomainSpec {
            id: "foo".into(),
            version: "1".into(),
            yaml_body: "id: foo".into(),
        })
        .await
        .unwrap();

    let list = client.list_domains(Empty {}).await.unwrap().into_inner();
    assert_eq!(list.domains.len(), 1);
    assert_eq!(list.domains[0].id, "foo");

    client
        .unregister_domain(DomainRef {
            id: "foo".into(),
            version: "1".into(),
        })
        .await
        .unwrap();

    let list = client.list_domains(Empty {}).await.unwrap().into_inner();
    assert!(list.domains.is_empty());
}

#[tokio::test]
async fn scheduler_and_discovery_round_trip() {
    let (addr, _shutdown) = start_server().await;
    let mut scheduler = SchedulerClient::connect(format!("http://{addr}")).await.unwrap();
    let mut discovery = DiscoveryClient::connect(format!("http://{addr}")).await.unwrap();

    scheduler
        .register_instance(InstanceInfo {
            instance_id: "i-1".into(),
            domain_id: "foo".into(),
            tags: vec!["blue".into()],
            region: "us-east".into(),
            capabilities: vec!["llm".into()],
        })
        .await
        .unwrap();

    let instances = scheduler
        .list_instances(DomainRef {
            id: "foo".into(),
            version: "1".into(),
        })
        .await
        .unwrap()
        .into_inner();
    assert_eq!(instances.instances.len(), 1);

    scheduler
        .heartbeat(InstanceRef {
            instance_id: "i-1".into(),
        })
        .await
        .unwrap();

    let found = discovery
        .discover(DiscoverRequest {
            domain_id: "foo".into(),
            tags: vec!["blue".into()],
            region: "us-east".into(),
        })
        .await
        .unwrap()
        .into_inner();
    assert_eq!(found.instances.len(), 1);
    assert_eq!(found.instances[0].instance_id, "i-1");

    scheduler
        .unregister_instance(InstanceRef {
            instance_id: "i-1".into(),
        })
        .await
        .unwrap();

    let found = discovery
        .discover(DiscoverRequest {
            domain_id: "foo".into(),
            tags: vec![],
            region: "".into(),
        })
        .await
        .unwrap()
        .into_inner();
    assert!(found.instances.is_empty());
}

#[tokio::test]
async fn telemetry_round_trip() {
    let (addr, _shutdown) = start_server().await;
    let mut client = TelemetryClient::connect(format!("http://{addr}")).await.unwrap();

    client
        .record_trace(TraceRecord {
            session_id: "s-1".into(),
            domain_id: "foo".into(),
            step_id: "step-1".into(),
            agent_id: "agent-1".into(),
            started_at_ms: 1,
            duration_ms: 10,
            input: "in".into(),
            output: "out".into(),
        })
        .await
        .unwrap();

    let traces = client
        .query_traces(QueryTraceRequest {
            domain_id: "foo".into(),
            session_id: "s-1".into(),
        })
        .await
        .unwrap()
        .into_inner();
    assert_eq!(traces.traces.len(), 1);
}
