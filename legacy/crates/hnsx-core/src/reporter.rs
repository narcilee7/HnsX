//! Telemetry reporters: local JSONL, gRPC control plane, and no-op.

use std::sync::Arc;

use async_trait::async_trait;
use hnsx_proto::v1::{InvocationRecord, TraceRecord, telemetry_client::TelemetryClient};
use tonic::transport::Channel;

use crate::error::{Error, Result};
use crate::telemetry::StepTrace;

/// Something that can forward telemetry to a persistent store.
#[async_trait]
pub trait Reporter: Send + Sync {
    /// Send a step trace.
    async fn report_trace(&self, trace: &StepTrace) -> Result<()>;
    /// Send an invocation summary.
    async fn report_invocation(&self, record: &InvocationRecord) -> Result<()>;
}

/// Reporter that discards everything. Useful for tests and local-only runs.
#[derive(Clone, Copy, Debug, Default)]
pub struct NoopReporter;

#[async_trait]
impl Reporter for NoopReporter {
    async fn report_trace(&self, _trace: &StepTrace) -> Result<()> {
        Ok(())
    }

    async fn report_invocation(&self, _record: &InvocationRecord) -> Result<()> {
        Ok(())
    }
}

/// Reporter that forwards telemetry to the HnsX control plane via gRPC.
#[derive(Clone, Debug)]
pub struct GrpcReporter {
    client: TelemetryClient<Channel>,
}

impl GrpcReporter {
    /// Connect to a control plane at `addr` (e.g. `http://127.0.0.1:50051`).
    ///
    /// # Errors
    ///
    /// Returns an error if the gRPC channel cannot be established.
    pub async fn connect(addr: &str) -> Result<Self> {
        let client = TelemetryClient::connect(addr.to_owned())
            .await
            .map_err(|e| Error::Adapter(format!("failed to connect to control plane: {e}")))?;
        Ok(Self { client })
    }

    /// Build a reporter from an existing gRPC client.
    pub fn new(client: TelemetryClient<Channel>) -> Self {
        Self { client }
    }
}

#[async_trait]
impl Reporter for GrpcReporter {
    async fn report_trace(&self, trace: &StepTrace) -> Result<()> {
        let req = tonic::Request::new(TraceRecord {
            session_id: trace.session_id.clone(),
            domain_id: trace.domain_id.clone(),
            step_id: trace.step_id.clone(),
            agent_id: trace.agent_id.clone(),
            started_at_ms: trace.started_at_ms as i64,
            duration_ms: trace.duration_ms as i64,
            input: serde_json::to_string(&trace.input)
                .map_err(|e| Error::Adapter(format!("serialise trace input: {e}")))?,
            output: trace.output.clone(),
        });
        self.client
            .clone()
            .record_trace(req)
            .await
            .map_err(|e| Error::Adapter(format!("record_trace failed: {e}")))?;
        Ok(())
    }

    async fn report_invocation(&self, record: &InvocationRecord) -> Result<()> {
        let req = tonic::Request::new(InvocationRecord {
            session_id: record.session_id.clone(),
            domain_id: record.domain_id.clone(),
            started_at_ms: record.started_at_ms,
            duration_ms: record.duration_ms,
            prompt_tokens: record.prompt_tokens,
            completion_tokens: record.completion_tokens,
            total_cost_usd: record.total_cost_usd,
        });
        self.client
            .clone()
            .record_invocation(req)
            .await
            .map_err(|e| Error::Adapter(format!("record_invocation failed: {e}")))?;
        Ok(())
    }
}

/// Type-erased boxed reporter.
pub type SharedReporter = Arc<dyn Reporter>;
