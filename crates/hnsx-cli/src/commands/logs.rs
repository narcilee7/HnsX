use anyhow::{Context, Result};
use clap::Args;
use hnsx_proto::v1::{
    QueryTraceRequest, telemetry_client::TelemetryClient,
};
use tonic::transport::Channel;

#[derive(Args, Debug)]
pub struct LogsArgs {
    /// Domain id to query.
    #[arg(long)]
    pub domain_id: String,
    /// Optional session id filter.
    #[arg(long)]
    pub session_id: Option<String>,
    /// Control plane gRPC address.
    #[arg(long, default_value = "http://127.0.0.1:50051")]
    pub control_plane: String,
    /// Follow log output (poll every 2 seconds).
    #[arg(long, short = 'f', default_value_t = false)]
    pub follow: bool,
}

pub fn exec(args: LogsArgs) -> Result<()> {
    let rt = tokio::runtime::Runtime::new().context("create tokio runtime")?;
    rt.block_on(run(args))
}

async fn run(args: LogsArgs) -> Result<()> {
    let mut client = TelemetryClient::<Channel>::connect(args.control_plane)
        .await
        .context("connect to control plane")?;

    let req = QueryTraceRequest {
        domain_id: args.domain_id.clone(),
        session_id: args.session_id.clone().unwrap_or_default(),
    };

    if args.follow {
        let mut seen = std::collections::HashSet::new();
        loop {
            let resp = client
                .query_traces(req.clone())
                .await
                .context("query traces")?
                .into_inner();
            for trace in resp.traces {
                let key = format!("{}:{}:{}:{}", trace.session_id, trace.step_id, trace.started_at_ms, trace.duration_ms);
                if seen.insert(key) {
                    print_log(&trace);
                }
            }
            tokio::time::sleep(std::time::Duration::from_secs(2)).await;
        }
    } else {
        let resp = client
            .query_traces(req)
            .await
            .context("query traces")?
            .into_inner();
        for trace in resp.traces {
            print_log(&trace);
        }
    }
    Ok(())
}

fn print_log(trace: &hnsx_proto::v1::TraceRecord) {
    println!(
        "[{}] session={} step={} agent={} duration={}ms input={} output={}",
        trace.started_at_ms,
        trace.session_id,
        trace.step_id,
        trace.agent_id,
        trace.duration_ms,
        trace.input,
        trace.output
    );
}
