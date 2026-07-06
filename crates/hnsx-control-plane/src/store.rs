//! Shared SQLite-backed state store for the control plane.
//!
//! All services (`Registry`, `Scheduler`, `Discovery`, `Telemetry`) use a single
//! `SqliteStore` instance so that state survives process restarts and can be
//! observed consistently across service boundaries.

use std::sync::Arc;

use rusqlite::{Connection, params};
use tokio::sync::Mutex;

use crate::proto::{DomainSpec, InstanceInfo, TraceRecord};

/// Central control-plane store backed by SQLite.
///
/// The connection is protected by an async mutex. All operations are synchronous
/// SQLite calls and therefore should remain short; they never cross an await
/// point while the guard is held.
#[derive(Clone)]
pub struct SqliteStore {
    conn: Arc<Mutex<Connection>>,
}

impl SqliteStore {
    /// Open a store at `path`. The parent directory is created if necessary.
    ///
    /// # Errors
    ///
    /// Returns an error if the directory cannot be created or the database
    /// cannot be opened.
    pub async fn open(path: &str) -> anyhow::Result<Self> {
        let path = path.to_owned();
        tokio::task::spawn_blocking(move || {
            let parent = std::path::Path::new(&path).parent().map(std::path::Path::to_owned);
            if let Some(parent) = parent {
                std::fs::create_dir_all(parent)?;
            }
            let conn = Connection::open(&path)?;
            let store = Self {
                conn: Arc::new(Mutex::new(conn)),
            };
            store.init_schema_blocking()?;
            Ok(store)
        })
        .await?
    }

    /// Create an in-memory store, useful for tests and ephemeral control planes.
    ///
    /// # Errors
    ///
    /// Returns an error if the in-memory database cannot be opened.
    pub async fn open_in_memory() -> anyhow::Result<Self> {
        tokio::task::spawn_blocking(|| {
            let conn = Connection::open_in_memory()?;
            let store = Self {
                conn: Arc::new(Mutex::new(conn)),
            };
            store.init_schema_blocking()?;
            Ok(store)
        })
        .await?
    }

    fn init_schema_blocking(&self) -> anyhow::Result<()> {
        let conn = self.conn.try_lock().map_err(|_| {
            anyhow::anyhow!("store connection is unexpectedly locked during initialization")
        })?;
        conn.execute_batch(
            "CREATE TABLE IF NOT EXISTS domains (
                id TEXT NOT NULL,
                version TEXT NOT NULL,
                yaml_body TEXT NOT NULL,
                PRIMARY KEY (id, version)
            );

            CREATE TABLE IF NOT EXISTS instances (
                instance_id TEXT PRIMARY KEY,
                domain_id TEXT NOT NULL,
                tags TEXT NOT NULL DEFAULT '[]',
                region TEXT NOT NULL DEFAULT '',
                capabilities TEXT NOT NULL DEFAULT '[]',
                last_seen_at_ms INTEGER NOT NULL
            );

            CREATE INDEX IF NOT EXISTS idx_instances_domain_id
                ON instances(domain_id);

            CREATE TABLE IF NOT EXISTS traces (
                session_id TEXT NOT NULL,
                domain_id TEXT NOT NULL,
                step_id TEXT NOT NULL,
                agent_id TEXT NOT NULL,
                started_at_ms INTEGER NOT NULL,
                duration_ms INTEGER NOT NULL,
                input TEXT NOT NULL DEFAULT '',
                output TEXT NOT NULL DEFAULT ''
            );

            CREATE INDEX IF NOT EXISTS idx_traces_domain_session
                ON traces(domain_id, session_id);
            ",
        )?;
        Ok(())
    }

    // ------------------------------------------------------------------
    // Registry
    // ------------------------------------------------------------------

    /// Insert or replace a domain spec.
    pub async fn register_domain(&self, spec: &DomainSpec) -> anyhow::Result<()> {
        let conn = self.conn.lock().await;
        conn.execute(
            "INSERT INTO domains (id, version, yaml_body)
             VALUES (?1, ?2, ?3)
             ON CONFLICT(id, version) DO UPDATE SET yaml_body = excluded.yaml_body",
            params![&spec.id, &spec.version, &spec.yaml_body],
        )?;
        Ok(())
    }

    /// Delete a domain by id and version.
    pub async fn unregister_domain(&self, id: &str, version: &str) -> anyhow::Result<()> {
        let conn = self.conn.lock().await;
        conn.execute(
            "DELETE FROM domains WHERE id = ?1 AND version = ?2",
            params![id, version],
        )?;
        Ok(())
    }

    /// Return all registered domains.
    pub async fn list_domains(&self) -> anyhow::Result<Vec<DomainSpec>> {
        let conn = self.conn.lock().await;
        let mut stmt = conn.prepare(
            "SELECT id, version, yaml_body FROM domains ORDER BY id, version",
        )?;
        let rows = stmt.query_map([], |row| {
            Ok(DomainSpec {
                id: row.get(0)?,
                version: row.get(1)?,
                yaml_body: row.get(2)?,
            })
        })?;
        rows.collect::<Result<Vec<_>, _>>()
            .map_err(Into::into)
    }

    // ------------------------------------------------------------------
    // Scheduler / Discovery
    // ------------------------------------------------------------------

    /// Insert or replace an instance record, refreshing its `last_seen_at_ms`.
    pub async fn register_instance(&self, info: &InstanceInfo) -> anyhow::Result<()> {
        let tags = serde_json::to_string(&info.tags)?;
        let caps = serde_json::to_string(&info.capabilities)?;
        let now_ms = chrono::Utc::now().timestamp_millis();
        let conn = self.conn.lock().await;
        conn.execute(
            "INSERT INTO instances
                (instance_id, domain_id, tags, region, capabilities, last_seen_at_ms)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6)
             ON CONFLICT(instance_id) DO UPDATE SET
                domain_id = excluded.domain_id,
                tags = excluded.tags,
                region = excluded.region,
                capabilities = excluded.capabilities,
                last_seen_at_ms = excluded.last_seen_at_ms",
            params![
                &info.instance_id,
                &info.domain_id,
                tags,
                &info.region,
                caps,
                now_ms,
            ],
        )?;
        Ok(())
    }

    /// Refresh the heartbeat timestamp for an instance.
    ///
    /// Returns `true` if the instance existed and was updated.
    pub async fn heartbeat(&self, instance_id: &str) -> anyhow::Result<bool> {
        let now_ms = chrono::Utc::now().timestamp_millis();
        let conn = self.conn.lock().await;
        let rows = conn.execute(
            "UPDATE instances SET last_seen_at_ms = ?1 WHERE instance_id = ?2",
            params![now_ms, instance_id],
        )?;
        Ok(rows > 0)
    }

    /// Remove an instance from the store.
    pub async fn unregister_instance(&self, instance_id: &str) -> anyhow::Result<()> {
        let conn = self.conn.lock().await;
        conn.execute(
            "DELETE FROM instances WHERE instance_id = ?1",
            params![instance_id],
        )?;
        Ok(())
    }

    /// Return all instances for a given domain.
    pub async fn list_instances(&self, domain_id: &str) -> anyhow::Result<Vec<InstanceInfo>> {
        let conn = self.conn.lock().await;
        let mut stmt = conn.prepare(
            "SELECT instance_id, domain_id, tags, region, capabilities
             FROM instances
             WHERE domain_id = ?1
             ORDER BY instance_id",
        )?;
        let rows = stmt.query_map(params![domain_id], |row| {
            let tags_json: String = row.get(2)?;
            let caps_json: String = row.get(4)?;
            Ok(InstanceInfo {
                instance_id: row.get(0)?,
                domain_id: row.get(1)?,
                tags: serde_json::from_str(&tags_json).unwrap_or_default(),
                region: row.get(3)?,
                capabilities: serde_json::from_str(&caps_json).unwrap_or_default(),
            })
        })?;
        rows.collect::<Result<Vec<_>, _>>()
            .map_err(Into::into)
    }

    /// Return instances matching domain, optional tags and optional region.
    ///
    /// All requested tags must be present on the instance. An empty filter
    /// matches every instance for the domain.
    pub async fn discover_instances(
        &self,
        domain_id: &str,
        tags: &[String],
        region: &str,
    ) -> anyhow::Result<Vec<InstanceInfo>> {
        let all: Vec<InstanceInfo> = self.list_instances(domain_id).await?;
        let region_nonempty = !region.is_empty();
        let tag_set: std::collections::HashSet<&str> = tags.iter().map(String::as_str).collect();
        Ok(all
            .into_iter()
            .filter(|info| {
                if region_nonempty && info.region != region {
                    return false;
                }
                if !tags.is_empty() {
                    let info_tags: std::collections::HashSet<&str> =
                        info.tags.iter().map(String::as_str).collect();
                    if !tag_set.is_subset(&info_tags) {
                        return false;
                    }
                }
                true
            })
            .collect())
    }

    /// Remove instances whose last heartbeat is older than `timeout_ms`.
    pub async fn expire_instances(&self, timeout_ms: i64) -> anyhow::Result<usize> {
        let cutoff = chrono::Utc::now().timestamp_millis() - timeout_ms;
        let conn = self.conn.lock().await;
        let rows = conn.execute(
            "DELETE FROM instances WHERE last_seen_at_ms < ?1",
            params![cutoff],
        )?;
        Ok(rows)
    }

    // ------------------------------------------------------------------
    // Telemetry
    // ------------------------------------------------------------------

    /// Persist a trace record.
    pub async fn record_trace(&self, trace: &TraceRecord) -> anyhow::Result<()> {
        let conn = self.conn.lock().await;
        conn.execute(
            "INSERT INTO traces
                (session_id, domain_id, step_id, agent_id,
                 started_at_ms, duration_ms, input, output)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
            params![
                &trace.session_id,
                &trace.domain_id,
                &trace.step_id,
                &trace.agent_id,
                trace.started_at_ms,
                trace.duration_ms,
                &trace.input,
                &trace.output,
            ],
        )?;
        Ok(())
    }

    /// Query traces by domain and optional session id.
    pub async fn query_traces(
        &self,
        domain_id: &str,
        session_id: Option<&str>,
    ) -> anyhow::Result<Vec<TraceRecord>> {
        let conn = self.conn.lock().await;
        if let Some(session) = session_id {
            let mut stmt = conn.prepare(
                "SELECT session_id, domain_id, step_id, agent_id,
                        started_at_ms, duration_ms, input, output
                 FROM traces
                 WHERE domain_id = ?1 AND session_id = ?2
                 ORDER BY started_at_ms, rowid",
            )?;
            let rows = stmt.query_map(params![domain_id, session], trace_from_row)?;
            rows.collect::<Result<Vec<_>, _>>()
                .map_err(Into::into)
        } else {
            let mut stmt = conn.prepare(
                "SELECT session_id, domain_id, step_id, agent_id,
                        started_at_ms, duration_ms, input, output
                 FROM traces
                 WHERE domain_id = ?1
                 ORDER BY started_at_ms, rowid",
            )?;
            let rows = stmt.query_map(params![domain_id], trace_from_row)?;
            rows.collect::<Result<Vec<_>, _>>()
                .map_err(Into::into)
        }
    }
}

fn trace_from_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<TraceRecord> {
    Ok(TraceRecord {
        session_id: row.get(0)?,
        domain_id: row.get(1)?,
        step_id: row.get(2)?,
        agent_id: row.get(3)?,
        started_at_ms: row.get(4)?,
        duration_ms: row.get(5)?,
        input: row.get(6)?,
        output: row.get(7)?,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn round_trip_domain() {
        let store = SqliteStore::open_in_memory().await.unwrap();
        let spec = DomainSpec {
            id: "foo".into(),
            version: "1".into(),
            yaml_body: "id: foo".into(),
        };
        store.register_domain(&spec).await.unwrap();
        let domains = store.list_domains().await.unwrap();
        assert_eq!(domains.len(), 1);
        assert_eq!(domains[0].id, "foo");

        store.unregister_domain("foo", "1").await.unwrap();
        assert!(store.list_domains().await.unwrap().is_empty());
    }

    #[tokio::test]
    async fn round_trip_instance() {
        let store = SqliteStore::open_in_memory().await.unwrap();
        let info = InstanceInfo {
            instance_id: "i-1".into(),
            domain_id: "foo".into(),
            tags: vec!["blue".into()],
            region: "us-east".into(),
            capabilities: vec!["llm".into()],
        };
        store.register_instance(&info).await.unwrap();

        let found = store.list_instances("foo").await.unwrap();
        assert_eq!(found.len(), 1);
        assert_eq!(found[0].instance_id, "i-1");

        assert!(store.heartbeat("i-1").await.unwrap());
        assert!(!store.heartbeat("missing").await.unwrap());

        let discovered = store
            .discover_instances("foo", &["blue".into()], "us-east")
            .await
            .unwrap();
        assert_eq!(discovered.len(), 1);

        store.unregister_instance("i-1").await.unwrap();
        assert!(store.list_instances("foo").await.unwrap().is_empty());
    }

    #[tokio::test]
    async fn round_trip_trace() {
        let store = SqliteStore::open_in_memory().await.unwrap();
        let trace = TraceRecord {
            session_id: "s-1".into(),
            domain_id: "foo".into(),
            step_id: "step-1".into(),
            agent_id: "agent-1".into(),
            started_at_ms: 1,
            duration_ms: 10,
            input: "in".into(),
            output: "out".into(),
        };
        store.record_trace(&trace).await.unwrap();

        let all = store.query_traces("foo", None).await.unwrap();
        assert_eq!(all.len(), 1);

        let by_session = store.query_traces("foo", Some("s-1")).await.unwrap();
        assert_eq!(by_session.len(), 1);

        let empty = store.query_traces("foo", Some("s-2")).await.unwrap();
        assert!(empty.is_empty());
    }
}
