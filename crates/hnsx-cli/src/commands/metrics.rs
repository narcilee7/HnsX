use anyhow::{Context, Result};
use clap::Args;
use hnsx_proto::v1::{
    InvocationMetricsRequest, telemetry_client::TelemetryClient,
};
use tonic::transport::Channel;

#[derive(Args, Debug)]
pub struct MetricsArgs {
    /// Domain id to query.
    #[arg(long)]
    pub domain_id: String,
    /// Control plane gRPC address.
    #[arg(long, default_value = "http://127.0.0.1:50051")]
    pub control_plane: String,
}

pub fn exec(args: MetricsArgs) -> Result<()> {
    let rt = tokio::runtime::Runtime::new().context("create tokio runtime")?;
    rt.block_on(run(args))
}

async fn run(args: MetricsArgs) -> Result<()> {
    let mut client = TelemetryClient::<Channel>::connect(args.control_plane)
        .await
        .context("connect to control plane")?;

    let req = InvocationMetricsRequest {
        domain_id: args.domain_id,
    };
    let m = client
        .query_invocation_metrics(req)
        .await
        .context("query invocation metrics")?
        .into_inner();

    println!("Invocation metrics for domain '{}'", m.domain_id);
    println!("  invocations: {}", m.invocation_count);
    println!("  avg_latency_ms: {:.2}", m.avg_latency_ms);
    println!("  total_prompt_tokens: {}", m.total_prompt_tokens);
    println!("  total_completion_tokens: {}", m.total_completion_tokens);
    println!(
        "  total_tokens: {}",
        m.total_prompt_tokens + m.total_completion_tokens
    );
    println!("  total_cost_usd: {:.6}", m.total_cost_usd);
    Ok(())
}
