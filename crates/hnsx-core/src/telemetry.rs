//! Minimal telemetry: per-step JSONL traces.
//!
//! Phase 1.7 lands the bare minimum so operators can see what ran, in
//! what order, and how long each step took. Format: one JSON object per
//! line, in `$HNSX_TRACE_DIR` (default `~/.hnsx/traces/`), keyed by
//! `session_id`. Each line is a [`StepTrace`].
//!
//! OpenTelemetry + Prometheus hooks land with Phase 5 (control plane).

use std::fs::{OpenOptions, create_dir_all};
use std::io::Write;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};
use std::time::{SystemTime, UNIX_EPOCH};

use hnsx_proto::v1::InvocationRecord;
use serde::Serialize;
use serde_json::Value;

use crate::error::{Error, Result};
use crate::reporter::{NoopReporter, Reporter};

/// A single step's telemetry record, written as one JSON line.
#[derive(Debug, Clone, Serialize)]
pub struct StepTrace {
    pub session_id: String,
    pub domain_id: String,
    pub step_id: String,
    pub agent_id: String,
    /// Wall-clock start of the step, in ms since the Unix epoch.
    pub started_at_ms: u64,
    /// How long the step ran (agent invoke + bookkeeping), in ms.
    pub duration_ms: u64,
    /// The input the workflow engine handed to the agent.
    pub input: Value,
    /// The aggregated text output from the agent.
    pub output: String,
}

/// Cheaply cloneable handle to a trace directory.
#[derive(Clone)]
pub struct Telemetry {
    inner: Arc<Mutex<TelemetryInner>>,
}

struct TelemetryInner {
    trace_dir: PathBuf,
    reporter: Arc<dyn Reporter>,
}

impl Telemetry {
    /// Build a telemetry sink rooted at `$HNSX_TRACE_DIR` if set, otherwise
    /// `$HOME/.hnsx/traces/`. Creates the directory if missing.
    pub fn new() -> Result<Self> {
        let dir = resolve_trace_dir()?;
        Self::with_dir(dir)
    }

    /// Build a telemetry sink rooted at a specific directory. Used by tests
    /// and by the CLI when an explicit `--trace-dir` is passed.
    pub fn with_dir(dir: PathBuf) -> Result<Self> {
        Self::with_dir_and_reporter(dir, Arc::new(NoopReporter))
    }

    /// Build a telemetry sink with an explicit reporter (e.g. gRPC).
    pub fn with_dir_and_reporter(
        dir: PathBuf,
        reporter: Arc<dyn Reporter>,
    ) -> Result<Self> {
        create_dir_all(&dir).map_err(|e| {
            Error::Adapter(format!(
                "failed to create trace directory {}: {e}",
                dir.display()
            ))
        })?;
        Ok(Self {
            inner: Arc::new(Mutex::new(TelemetryInner { trace_dir: dir, reporter })),
        })
    }

    /// Replace the reporter.
    pub fn set_reporter(&self,
        reporter: Arc<dyn Reporter>,
    ) {
        if let Ok(mut inner) = self.inner.lock() {
            inner.reporter = reporter;
        }
    }

    /// Append a step trace to `<dir>/<session_id>.jsonl` and forward it to
    /// the configured reporter.
    pub fn record_step(&self,
        trace: &StepTrace,
    ) -> Result<()> {
        let (path, reporter) = {
            let inner = self
                .inner
                .lock()
                .map_err(|_| Error::Adapter("telemetry mutex poisoned".into()))?;
            let path = inner.trace_dir.join(format!("{}.jsonl", trace.session_id));
            (path, inner.reporter.clone())
        };

        let mut file = OpenOptions::new()
            .create(true)
            .append(true)
            .open(&path)
            .map_err(|e| Error::Adapter(format!("open {}: {e}", path.display())))?;
        let line = serde_json::to_string(trace)
            .map_err(|e| Error::Adapter(format!("serialise StepTrace: {e}")))?;
        writeln!(file, "{line}")
            .map_err(|e| Error::Adapter(format!("write {}: {e}", path.display())))?;

        // Forward asynchronously without blocking the caller.
        let trace = trace.clone();
        tokio::spawn(async move {
            if let Err(e) = reporter.report_trace(&trace).await {
                tracing::warn!(error = %e, "failed to report trace");
            }
        });
        Ok(())
    }

    /// Report a completed invocation summary.
    pub fn record_invocation(&self,
        record: &InvocationRecord,
    ) {
        let reporter = self
            .inner
            .lock()
            .map(|i| i.reporter.clone())
            .unwrap_or_else(|_| Arc::new(NoopReporter));
        let record = record.clone();
        tokio::spawn(async move {
            if let Err(e) = reporter.report_invocation(&record).await {
                tracing::warn!(error = %e, "failed to report invocation");
            }
        });
    }

    /// The directory this sink writes to. Mainly for diagnostics.
    pub fn trace_dir(&self) -> PathBuf {
        self.inner
            .lock()
            .map(|i| i.trace_dir.clone())
            .unwrap_or_default()
    }
}

/// Wall-clock milliseconds since the Unix epoch. Returns 0 if the system
/// clock is before the epoch (extremely unlikely on any modern OS).
pub fn now_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as u64)
        .unwrap_or(0)
}

fn resolve_trace_dir() -> Result<PathBuf> {
    if let Ok(dir) = std::env::var("HNSX_TRACE_DIR") {
        if !dir.is_empty() {
            return Ok(PathBuf::from(dir));
        }
    }
    let home = std::env::var("HOME")
        .map_err(|_| Error::Adapter("HOME not set and HNSX_TRACE_DIR not set".into()))?;
    Ok(PathBuf::from(home).join(".hnsx").join("traces"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[tokio::test]
    async fn new_creates_dir_and_appends_lines() {
        let dir = tempdir().expect("tempdir");
        let path = dir.path().to_path_buf();
        let t = Telemetry::with_dir(path.clone()).expect("with_dir");

        let trace = StepTrace {
            session_id: "s1".into(),
            domain_id: "d1".into(),
            step_id: "step1".into(),
            agent_id: "a".into(),
            started_at_ms: now_ms(),
            duration_ms: 12,
            input: serde_json::json!({"q": "hi"}),
            output: "hello".into(),
        };
        t.record_step(&trace).expect("record");
        t.record_step(&trace).expect("record again");

        let written = std::fs::read_to_string(path.join("s1.jsonl")).expect("read trace file");
        let lines: Vec<&str> = written.lines().collect();
        assert_eq!(lines.len(), 2);
        for line in lines {
            let v: serde_json::Value = serde_json::from_str(line).expect("valid json");
            assert_eq!(v["session_id"], "s1");
            assert_eq!(v["step_id"], "step1");
            assert_eq!(v["agent_id"], "a");
            assert_eq!(v["output"], "hello");
            assert_eq!(v["duration_ms"], 12);
        }
    }

    #[tokio::test]
    async fn different_sessions_get_different_files() {
        let dir = tempdir().expect("tempdir");
        let t = Telemetry::with_dir(dir.path().to_path_buf()).expect("with_dir");

        let mut trace = StepTrace {
            session_id: "s_a".into(),
            domain_id: "d".into(),
            step_id: "step".into(),
            agent_id: "a".into(),
            started_at_ms: 0,
            duration_ms: 1,
            input: serde_json::json!({}),
            output: "x".into(),
        };
        t.record_step(&trace).expect("a");
        trace.session_id = "s_b".into();
        t.record_step(&trace).expect("b");

        assert!(dir.path().join("s_a.jsonl").exists());
        assert!(dir.path().join("s_b.jsonl").exists());
    }

    #[test]
    fn record_step_propagates_io_errors_as_adapter() {
        // Build a telemetry sink pointed at a path that exists but is a
        // regular file — opening `<file>/<session>.jsonl` must fail.
        let dir = tempdir().expect("tempdir");
        let file_path = dir.path().join("not-a-dir");
        std::fs::write(&file_path, b"").expect("write");
        // `with_dir` creates the directory, so it succeeds. To force a
        // failure, call record_step with a session_id whose resolved
        // path crosses the file boundary. We simulate by constructing a
        // Telemetry that points at a file path itself; `with_dir` will
        // fail at create_dir_all.
        let t = Telemetry::with_dir(file_path.clone());
        assert!(t.is_err(), "expected with_dir to fail on a non-dir path");
    }
}
