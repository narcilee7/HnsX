//! Trace and metrics aggregation service.
//!
//! Stores execution traces in the shared `SqliteStore` and allows querying by
//! domain and optional session id.

use tonic::{Request, Response, Status};

use crate::{
    proto::{
        Empty, InvocationMetrics, InvocationMetricsRequest, InvocationRecord,
        QueryTraceRequest, TraceList, TraceRecord, telemetry_server::Telemetry,
    },
    store::SqliteStore,
};

#[derive(Clone)]
pub struct TelemetryService {
    store: SqliteStore,
}

impl TelemetryService {
    pub fn new(store: SqliteStore) -> Self {
        Self { store }
    }
}

#[tonic::async_trait]
impl Telemetry for TelemetryService {
    async fn record_trace(
        &self,
        request: Request<TraceRecord>,
    ) -> Result<Response<Empty>, Status> {
        crate::timed_grpc_async!("record_trace", async {
            let trace = request.into_inner();
            self.store
                .record_trace(&trace)
                .await
                .map_err(|e| Status::internal(format!("failed to record trace: {e}")))?;
            Ok(Response::new(Empty {}))
        })
    }

    async fn query_traces(
        &self,
        request: Request<QueryTraceRequest>,
    ) -> Result<Response<TraceList>, Status> {
        crate::timed_grpc_async!("query_traces", async {
            let req = request.into_inner();
            let session_id = if req.session_id.is_empty() {
                None
            } else {
                Some(req.session_id.as_str())
            };
            let traces = self
                .store
                .query_traces(&req.domain_id, session_id)
                .await
                .map_err(|e| Status::internal(format!("failed to query traces: {e}")))?;
            Ok(Response::new(TraceList { traces }))
        })
    }

    async fn record_invocation(
        &self,
        request: Request<InvocationRecord>,
    ) -> Result<Response<Empty>, Status> {
        crate::timed_grpc_async!("record_invocation", async {
            let record = request.into_inner();
            self.store
                .record_invocation(&record)
                .await
                .map_err(|e| Status::internal(format!("failed to record invocation: {e}")))?;
            crate::metrics::record_invocation(&record.domain_id, record.duration_ms as f64);
            crate::metrics::record_usage(
                &record.domain_id,
                record.prompt_tokens as u64,
                record.completion_tokens as u64,
                record.total_cost_usd,
            );
            Ok(Response::new(Empty {}))
        })
    }

    async fn query_invocation_metrics(
        &self,
        request: Request<InvocationMetricsRequest>,
    ) -> Result<Response<InvocationMetrics>, Status> {
        crate::timed_grpc_async!("query_invocation_metrics", async {
            let req = request.into_inner();
            let metrics = self
                .store
                .query_invocation_metrics(&req.domain_id)
                .await
                .map_err(|e| Status::internal(format!("failed to query metrics: {e}")))?;
            Ok(Response::new(metrics))
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn record_then_query() {
        let store = SqliteStore::open_in_memory().await.unwrap();
        let svc = TelemetryService::new(store);
        svc.record_trace(Request::new(TraceRecord {
            session_id: "s-1".into(),
            domain_id: "foo".into(),
            step_id: "step-1".into(),
            agent_id: "agent-1".into(),
            started_at_ms: 1,
            duration_ms: 10,
            input: "in".into(),
            output: "out".into(),
        }))
        .await
        .unwrap();

        let resp = svc
            .query_traces(Request::new(QueryTraceRequest {
                domain_id: "foo".into(),
                session_id: "s-1".into(),
            }))
            .await
            .unwrap();
        assert_eq!(resp.into_inner().traces.len(), 1);
    }
}
