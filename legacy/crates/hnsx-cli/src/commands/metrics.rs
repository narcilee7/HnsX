use anyhow::{Context, Result};
use clap::Args;
use hnsx_proto::v1::{InvocationMetricsRequest, telemetry_client::TelemetryClient};
use std::path::PathBuf;
use tonic::transport::Channel;

#[derive(Args, Debug)]
pub struct MetricsArgs {
    /// Domain id to filter by (optional in local mode).
    #[arg(long)]
    pub domain_id: Option<String>,
    /// Control plane gRPC address. If set, metrics are queried from the
    /// control plane instead of local trace files.
    #[arg(long)]
    pub control_plane: Option<String>,
    /// Directory containing JSONL trace files. Defaults to `$HNSX_TRACE_DIR`
    /// or `$HOME/.hnsx/traces`.
    #[arg(long)]
    pub trace_dir: Option<PathBuf>,
}

pub fn exec(args: MetricsArgs) -> Result<()> {
    let rt = tokio::runtime::Runtime::new().context("create tokio runtime")?;
    rt.block_on(run(args))
}

async fn run(args: MetricsArgs) -> Result<()> {
    if let Some(addr) = args.control_plane {
        return query_control_plane(addr, args.domain_id).await;
    }
    query_local_traces(args.trace_dir, args.domain_id.as_deref())
}

async fn query_control_plane(addr: String, domain_id: Option<String>) -> Result<()> {
    let mut client = TelemetryClient::<Channel>::connect(addr)
        .await
        .context("connect to control plane")?;

    let req = InvocationMetricsRequest {
        domain_id: domain_id.unwrap_or_default(),
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

fn query_local_traces(trace_dir: Option<PathBuf>, domain_id: Option<&str>) -> Result<()> {
    let dir = trace_dir.unwrap_or_else(default_trace_dir);
    let records = read_trace_lines(&dir, domain_id)?;

    if records.is_empty() {
        println!("No local traces found in {}.", dir.display());
        return Ok(());
    }

    let count = records.len() as u64;
    let total_duration_ms: u64 = records
        .iter()
        .filter_map(|v| v.get("duration_ms").and_then(|d| d.as_u64()))
        .sum();
    let avg_duration_ms = if count > 0 {
        total_duration_ms as f64 / count as f64
    } else {
        0.0
    };
    let total_prompt_tokens: u64 = records
        .iter()
        .filter_map(|v| v.get("prompt_tokens").and_then(|d| d.as_u64()))
        .sum();
    let total_completion_tokens: u64 = records
        .iter()
        .filter_map(|v| v.get("completion_tokens").and_then(|d| d.as_u64()))
        .sum();

    println!("Local trace metrics from {}", dir.display());
    if let Some(did) = domain_id {
        println!("  domain filter: {did}");
    }
    println!("  step_records: {}", count);
    println!("  total_duration_ms: {}", total_duration_ms);
    println!("  avg_duration_ms: {:.2}", avg_duration_ms);
    println!("  total_prompt_tokens: {}", total_prompt_tokens);
    println!("  total_completion_tokens: {}", total_completion_tokens);
    println!(
        "  total_tokens: {}",
        total_prompt_tokens + total_completion_tokens
    );
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

fn read_trace_lines(dir: &PathBuf, domain_id: Option<&str>) -> Result<Vec<serde_json::Value>> {
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
            records.push(value);
        }
    }
    Ok(records)
}
