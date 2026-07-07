use anyhow::{Context, Result};
use clap::Args;
use hnsx_proto::v1::{QueryTraceRequest, telemetry_client::TelemetryClient};
use std::path::PathBuf;
use tonic::transport::Channel;

#[derive(Args, Debug)]
pub struct TracesArgs {
    /// Domain id to filter by.
    #[arg(long)]
    pub domain_id: Option<String>,
    /// Optional session id filter.
    #[arg(long)]
    pub session_id: Option<String>,
    /// Control plane gRPC address. If set, traces are queried from the
    /// control plane instead of local trace files.
    #[arg(long)]
    pub control_plane: Option<String>,
    /// Directory containing JSONL trace files. Defaults to `$HNSX_TRACE_DIR`
    /// or `$HOME/.hnsx/traces`.
    #[arg(long)]
    pub trace_dir: Option<PathBuf>,
}

pub fn exec(args: TracesArgs) -> Result<()> {
    let rt = tokio::runtime::Runtime::new().context("create tokio runtime")?;
    rt.block_on(run(args))
}

async fn run(args: TracesArgs) -> Result<()> {
    if let Some(addr) = args.control_plane {
        return query_control_plane(addr, args.domain_id, args.session_id).await;
    }
    query_local_traces(
        args.trace_dir,
        args.domain_id.as_deref(),
        args.session_id.as_deref(),
    )
}

async fn query_control_plane(
    addr: String,
    domain_id: Option<String>,
    session_id: Option<String>,
) -> Result<()> {
    let mut client = TelemetryClient::<Channel>::connect(addr)
        .await
        .context("connect to control plane")?;

    let req = QueryTraceRequest {
        domain_id: domain_id.unwrap_or_default(),
        session_id: session_id.unwrap_or_default(),
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
            trace.started_at_ms, trace.session_id, trace.step_id, trace.agent_id, trace.duration_ms
        );
        println!("  input: {}", trace.input);
        println!("  output: {}", trace.output);
    }
    Ok(())
}

fn query_local_traces(
    trace_dir: Option<PathBuf>,
    domain_id: Option<&str>,
    session_id: Option<&str>,
) -> Result<()> {
    let dir = trace_dir.unwrap_or_else(default_trace_dir);
    let records = read_trace_lines(&dir, domain_id, session_id)?;

    if records.is_empty() {
        println!("No local traces found in {}.", dir.display());
        return Ok(());
    }

    for value in records {
        let started = value
            .get("started_at_ms")
            .and_then(|v| v.as_u64())
            .unwrap_or(0);
        let session = value
            .get("session_id")
            .and_then(|v| v.as_str())
            .unwrap_or("?");
        let step = value.get("step_id").and_then(|v| v.as_str()).unwrap_or("?");
        let agent = value
            .get("agent_id")
            .and_then(|v| v.as_str())
            .unwrap_or("?");
        let duration = value
            .get("duration_ms")
            .and_then(|v| v.as_u64())
            .unwrap_or(0);
        let input = value.get("input").unwrap_or(&serde_json::Value::Null);
        let output = value.get("output").and_then(|v| v.as_str()).unwrap_or("");

        println!(
            "[{}] session={} step={} agent={} duration={}ms",
            started, session, step, agent, duration
        );
        println!(
            "  input: {}",
            serde_json::to_string(input).unwrap_or_default()
        );
        println!("  output: {}", output);
    }
    Ok(())
}

fn default_trace_dir() -> PathBuf {
    if let Ok(dir) = std::env::var("HNSX_TRACE_DIR") {
        if !dir.is_empty() {
            return PathBuf::from(dir);
        }
    }
    let home = std::env::var("HOME").unwrap_or_else(|_| ".".into());
    PathBuf::from(home).join(".hnsx").join("traces")
}

fn read_trace_lines(
    dir: &PathBuf,
    domain_id: Option<&str>,
    session_id: Option<&str>,
) -> Result<Vec<serde_json::Value>> {
    if !dir.exists() {
        return Ok(Vec::new());
    }

    let mut records = Vec::new();
    for entry in
        std::fs::read_dir(dir).with_context(|| format!("read trace directory {}", dir.display()))?
    {
        let entry = entry?;
        let path = entry.path();
        if !path.is_file() || path.extension().and_then(|s| s.to_str()) != Some("jsonl") {
            continue;
        }
        let contents = std::fs::read_to_string(&path)
            .with_context(|| format!("read trace file {}", path.display()))?;
        for (line_no, line) in contents.lines().enumerate() {
            if line.trim().is_empty() {
                continue;
            }
            let value: serde_json::Value = serde_json::from_str(line).with_context(|| {
                format!("parse JSONL in {} line {}", path.display(), line_no + 1)
            })?;
            if let Some(did) = domain_id {
                if value.get("domain_id").and_then(|v| v.as_str()) != Some(did) {
                    continue;
                }
            }
            if let Some(sid) = session_id {
                if value.get("session_id").and_then(|v| v.as_str()) != Some(sid) {
                    continue;
                }
            }
            records.push(value);
        }
    }
    Ok(records)
}
