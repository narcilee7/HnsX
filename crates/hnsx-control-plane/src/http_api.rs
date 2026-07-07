//! REST API for the Web UI and CLI helpers.
//!
//! Provides JSON endpoints that wrap the shared `SqliteStore`. These are
//! easier to consume from browsers than gRPC directly.

use axum::{
    Json, Router,
    extract::{Path, State},
    http::StatusCode,
    response::IntoResponse,
    routing::get,
};
use serde::Serialize;

use crate::store::SqliteStore;

/// Shared application state for the HTTP API.
#[derive(Clone)]
pub struct AppState {
    pub store: SqliteStore,
}

impl AppState {
    /// Create state wrapping the given store.
    pub fn new(store: SqliteStore) -> Self {
        Self { store }
    }
}

/// Build a complete REST API router around the given store.
pub fn router(store: SqliteStore) -> Router {
    Router::new()
        .route("/api/v1/domains", get(list_domains))
        .route("/api/v1/instances/{domain_id}", get(list_instances))
        .route("/api/v1/traces/{domain_id}", get(list_traces))
        .route("/api/v1/metrics/{domain_id}", get(domain_metrics))
        .with_state(AppState::new(store))
}

#[derive(Serialize)]
struct DomainView {
    id: String,
    version: String,
    yaml_body: String,
}

async fn list_domains(State(state): State<AppState>) -> Result<Json<Vec<DomainView>>, AppError> {
    let domains = state.store.list_domains().await?;
    Ok(Json(
        domains
            .into_iter()
            .map(|d| DomainView {
                id: d.id,
                version: d.version,
                yaml_body: d.yaml_body,
            })
            .collect(),
    ))
}

#[axum::debug_handler]
async fn list_instances(
    State(state): State<AppState>,
    Path(domain_id): Path<String>,
) -> Result<Json<Vec<crate::proto::InstanceInfo>>, AppError> {
    let instances = state.store.list_instances(&domain_id).await?;
    Ok(Json(instances))
}

async fn list_traces(
    State(state): State<AppState>,
    Path(domain_id): Path<String>,
) -> Result<Json<Vec<crate::proto::TraceRecord>>, AppError> {
    let traces = state.store.query_traces(&domain_id, None).await?;
    Ok(Json(traces))
}

async fn domain_metrics(
    State(state): State<AppState>,
    Path(domain_id): Path<String>,
) -> Result<Json<crate::proto::InvocationMetrics>, AppError> {
    let metrics = state.store.query_invocation_metrics(&domain_id).await?;
    Ok(Json(metrics))
}

struct AppError(anyhow::Error);

impl From<anyhow::Error> for AppError {
    fn from(err: anyhow::Error) -> Self {
        Self(err)
    }
}

impl IntoResponse for AppError {
    fn into_response(self) -> axum::response::Response {
        (StatusCode::INTERNAL_SERVER_ERROR, format!("internal error: {}", self.0)).into_response()
    }
}
