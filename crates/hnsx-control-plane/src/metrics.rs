//! Prometheus-style metrics for the control plane.
//!
//! Records counters and histograms around gRPC calls and aggregates
//! invocation-level telemetry from the SQLite store.

use metrics::{Unit, counter, describe_counter, describe_gauge, describe_histogram, gauge, histogram};
use metrics_exporter_prometheus::PrometheusBuilder;

/// Initialize the global metrics recorder and return an HTTP handler that
/// renders Prometheus exposition format.
///
/// # Errors
///
/// Returns an error if the Prometheus builder fails to install.
pub fn install() -> anyhow::Result<metrics_exporter_prometheus::PrometheusHandle> {
    let builder = PrometheusBuilder::new();
    let handle = builder.install_recorder()?;

    describe_counter!(
        "hnsx_grpc_requests_total",
        "Total number of gRPC requests handled by the control plane"
    );
    describe_histogram!(
        "hnsx_grpc_request_duration_ms",
        Unit::Milliseconds,
        "gRPC request latency in milliseconds"
    );
    describe_counter!(
        "hnsx_invocations_total",
        "Total number of domain invocations recorded"
    );
    describe_histogram!(
        "hnsx_invocation_duration_ms",
        Unit::Milliseconds,
        "Domain invocation latency in milliseconds"
    );
    describe_gauge!(
        "hnsx_instances",
        "Number of registered agent instances"
    );
    describe_counter!(
        "hnsx_tokens_total",
        "Total number of tokens consumed"
    );
    describe_counter!(
        "hnsx_cost_usd_total",
        "Total estimated cost in USD"
    );

    Ok(handle)
}

/// Record a completed gRPC request.
pub fn record_grpc_request(method: &str, duration_ms: f64) {
    counter!("hnsx_grpc_requests_total", "method" => method.to_owned()).increment(1);
    histogram!(
        "hnsx_grpc_request_duration_ms",
        "method" => method.to_owned()
    )
    .record(duration_ms);
}

/// Record a domain invocation.
pub fn record_invocation(domain_id: &str, duration_ms: f64) {
    counter!("hnsx_invocations_total", "domain_id" => domain_id.to_owned()).increment(1);
    histogram!(
        "hnsx_invocation_duration_ms",
        "domain_id" => domain_id.to_owned()
    )
    .record(duration_ms);
}

/// Record token and cost counters.
pub fn record_usage(domain_id: &str, prompt_tokens: u64, completion_tokens: u64, cost_usd: f64) {
    counter!(
        "hnsx_tokens_total",
        "domain_id" => domain_id.to_owned(),
        "kind" => "prompt"
    )
    .increment(prompt_tokens);
    counter!(
        "hnsx_tokens_total",
        "domain_id" => domain_id.to_owned(),
        "kind" => "completion"
    )
    .increment(completion_tokens);
    counter!(
        "hnsx_cost_usd_total",
        "domain_id" => domain_id.to_owned()
    )
    .increment(cost_usd as u64);
}

/// Report current instance count.
pub fn set_instance_count(domain_id: &str, count: usize) {
    gauge!("hnsx_instances", "domain_id" => domain_id.to_owned()).set(count as f64);
}

/// Time a gRPC handler and record its metrics.
#[macro_export]
macro_rules! timed_grpc {
    ($method:expr, $body:expr) => {{
        let __start = std::time::Instant::now();
        let __result = $body;
        let __duration_ms = __start.elapsed().as_secs_f64() * 1000.0;
        $crate::metrics::record_grpc_request($method, __duration_ms);
        __result
    }};
}

/// Time an async gRPC handler and record its metrics.
#[macro_export]
macro_rules! timed_grpc_async {
    ($method:expr, $body:expr) => {{
        let __start = std::time::Instant::now();
        let __result = $body.await;
        let __duration_ms = __start.elapsed().as_secs_f64() * 1000.0;
        $crate::metrics::record_grpc_request($method, __duration_ms);
        __result
    }};
}
