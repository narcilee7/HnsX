use anyhow::{Context, Result};
use clap::Args;
use hnsx_proto::v1::{
    QueryTraceRequest, telemetry_client::TelemetryClient,
};
use tonic::transport::Channel;

#[derive(Args, Debug)]
pub struct TracesArgs {
    /// Domain id to query.
    #[arg(long)]
    pub domain_id: String,
    /// Optional session id filter.
    #[arg(long)]
    pub session_id: Option<String>,
    /// Control plane gRPC address.
    #[arg(long, default_value = "http://127.0.0.1:50051")]
    pub control_plane: String,
}

pub fn exec(args: TracesArgs) -> Result<()> {
    let rt = tokio::runtime::Runtime::new().context("create tokio runtime")?;
    rt.block_on(run(args))
}

async fn run(args: TracesArgs) -> Result<()> {
    let mut client = TelemetryClient::<Channel>::connect(args.control_plane)
        .await
        .context("connect to control plane")?;

    let req = QueryTraceRequest {
        domain_id: args.domain_id,
        session_id: args.session_id.unwrap_or_default(),
    };
    let resp = client
        .query_traces(req)
        .await
        .context("query traces")?
        .into_inner();

    if resp.traces.is_empty() {
        println!("No traces found.");
        return Ok(());
    }

    for trace in resp.traces {
        println!(
            "[{}] session={} step={} agent={} duration={}ms",
            trace.started_at_ms,
            trace.session_id,
            trace.step_id,
            trace.agent_id,
            trace.duration_ms
        );
        println!("  input: {}", trace.input);
        println!("  output: {}", trace.output);
    }
    Ok(())
}
