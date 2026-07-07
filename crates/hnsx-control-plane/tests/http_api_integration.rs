//! Integration test for the control-plane HTTP REST API.
//!
//! Starts the full server (gRPC + HTTP) on random ports and exercises the
//! endpoints consumed by the Web UI.

use std::net::SocketAddr;

use hnsx_control_plane::{
    proto::{
        DomainSpec, TraceRecord, registry_client::RegistryClient, telemetry_client::TelemetryClient,
    },
    server::ControlPlaneServer,
    store::SqliteStore,
};

async fn start_server() -> (SocketAddr, SocketAddr, tokio::sync::oneshot::Sender<()>) {
    let store = SqliteStore::open_in_memory().await.unwrap();

    // Bind both listeners in this task to avoid port races with parallel tests.
    let grpc_listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let grpc_addr = grpc_listener.local_addr().unwrap();
    let http_listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let http_addr = http_listener.local_addr().unwrap();

    let (shutdown_tx, mut shutdown_rx) = tokio::sync::oneshot::channel();
    tokio::spawn(async move {
        let server = ControlPlaneServer::new(store);
        let serve = server.serve_with_both_bound(grpc_listener, http_listener);
        tokio::select! {
            _ = serve => {},
            _ = &mut shutdown_rx => {},
        }
    });

    // Wait for the server to start serving.
    tokio::time::sleep(std::time::Duration::from_millis(50)).await;
    (grpc_addr, http_addr, shutdown_tx)
}

#[tokio::test]
async fn http_lists_domains_and_traces() {
    let (grpc_addr, http_addr, _shutdown) = start_server().await;
    let grpc_url = format!("http://{grpc_addr}");

    // Register a domain via gRPC.
    let mut registry = RegistryClient::connect(grpc_url.clone()).await.unwrap();
    registry
        .register_domain(DomainSpec {
            id: "ui-domain".into(),
            version: "1".into(),
            yaml_body: "id: ui-domain\nversion: '1'".into(),
        })
        .await
        .unwrap();

    // Record traces for two sessions via gRPC.
    let mut telemetry = TelemetryClient::connect(grpc_url).await.unwrap();
    for (session, step) in [("s-a", "step-1"), ("s-a", "step-2"), ("s-b", "step-1")] {
        telemetry
            .record_trace(TraceRecord {
                session_id: session.into(),
                domain_id: "ui-domain".into(),
                step_id: step.into(),
                agent_id: "agent-1".into(),
                started_at_ms: 1,
                duration_ms: 10,
                input: "{}".into(),
                output: "ok".into(),
            })
            .await
            .unwrap();
    }

    let http_base = format!("http://{http_addr}/api/v1");
    let client = reqwest::Client::new();

    // List domains.
    let domains = client
        .get(format!("{http_base}/domains"))
        .send()
        .await
        .unwrap()
        .json::<serde_json::Value>()
        .await
        .unwrap();
    assert_eq!(domains.as_array().unwrap().len(), 1);
    assert_eq!(domains[0]["id"], "ui-domain");

    // List sessions.
    let sessions = client
        .get(format!("{http_base}/sessions/ui-domain"))
        .send()
        .await
        .unwrap()
        .json::<serde_json::Value>()
        .await
        .unwrap();
    let sessions_arr = sessions.as_array().unwrap();
    assert_eq!(sessions_arr.len(), 2);

    // Filter traces by session.
    let traces = client
        .get(format!("{http_base}/traces/ui-domain?session_id=s-a"))
        .send()
        .await
        .unwrap()
        .json::<serde_json::Value>()
        .await
        .unwrap();
    let traces_arr = traces.as_array().unwrap();
    assert_eq!(traces_arr.len(), 2);
    assert_eq!(traces_arr[0]["session_id"], "s-a");
}

#[tokio::test]
async fn http_cors_allows_any_origin() {
    let (_grpc_addr, http_addr, _shutdown) = start_server().await;
    let mut stream = tokio::net::TcpStream::connect(http_addr).await.unwrap();
    let request =
        b"GET /api/v1/domains HTTP/1.1\r\nHost: localhost\r\nOrigin: http://example.com\r\n\r\n";
    tokio::io::AsyncWriteExt::write_all(&mut stream, request)
        .await
        .unwrap();
    let mut buf = vec![0u8; 1024];
    let n = tokio::io::AsyncReadExt::read(&mut stream, &mut buf)
        .await
        .unwrap();
    let response = String::from_utf8_lossy(&buf[..n]);
    println!("raw response:\n{response}");
    assert!(response.contains("HTTP/1.1 200 OK"), "response: {response}");
    assert!(
        response
            .to_lowercase()
            .contains("access-control-allow-origin: *"),
        "response: {response}"
    );
}
