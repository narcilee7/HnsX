//! Memory backend trait, session types, and multiple backend implementations.
//!
//! Backends:
//! - `InMemoryBackend`: default, process-local HashMap.
//! - `SqliteBackend`: persistent, file-backed SQLite.
//! - `RedisBackend`: Redis-backed (placeholder; returns `Unimplemented` in v1).
//! - `PostgresBackend`: Postgres-backed (placeholder; returns `Unimplemented` in v1).
//!
//! Use `MemoryBackendFactory::create(&MemoryConfig)` to obtain a backend.

use std::collections::HashMap;
use std::sync::{Arc, Mutex};

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::error::{Error, Result};

#[async_trait]
pub trait MemoryBackend: Send + Sync {
    async fn load_session(&self,
        domain_id: &str,
        session_id: &str,
    ) -> Result<Session>;
    async fn save_turn(
        &self,
        session: &Session,
        agent_id: &str,
        role: &str,
        content: &str,
    ) -> Result<()>;
    async fn build_context(
        &self,
        session: &Session,
        agent_id: &str,
        window: usize,
    ) -> Result<Vec<Message>>;
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Session {
    pub domain_id: String,
    pub session_id: String,
    #[serde(default)]
    pub turns: Vec<Turn>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Turn {
    pub agent_id: String,
    pub role: String,
    pub content: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub role: String,
    pub content: String,
}

// ---------------------------------------------------------------------------
// MemoryConfig
// ---------------------------------------------------------------------------

/// Top-level memory configuration attached to a Domain.
///
/// Supports either a plain string (`memory: in_memory`) or an object:
/// ```yaml
/// memory:
///   backend: sqlite
///   path: ./sessions.db
/// ```
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum MemoryConfig {
    /// Short form: just the backend kind name.
    Short(String),
    /// Structured form: backend kind plus backend-specific options.
    Structured {
        backend: String,
        #[serde(flatten)]
        options: Value,
    },
}

impl MemoryConfig {
    pub fn backend_kind(&self) -> &str {
        match self {
            MemoryConfig::Short(s) => s.as_str(),
            MemoryConfig::Structured { backend, .. } => backend.as_str(),
        }
    }

    pub fn options(&self) -> &Value {
        match self {
            MemoryConfig::Short(_) => &Value::Null,
            MemoryConfig::Structured { options, .. } => options,
        }
    }
}

impl Default for MemoryConfig {
    fn default() -> Self {
        MemoryConfig::Short("in_memory".into())
    }
}

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

/// Creates a `MemoryBackend` from a `MemoryConfig`.
pub struct MemoryBackendFactory;

impl MemoryBackendFactory {
    pub fn create(config: &MemoryConfig) -> Result<Arc<dyn MemoryBackend>> {
        let kind = config.backend_kind();
        match kind {
            "in_memory" => Ok(Arc::new(InMemoryBackend::new())),
            "sqlite" => {
                let path = config
                    .options()
                    .get("path")
                    .and_then(Value::as_str)
                    .unwrap_or("hnsx_sessions.db");
                Ok(Arc::new(SqliteBackend::new(path)?))
            }
            "redis" => Ok(Arc::new(RedisBackend::new(config.options())?)),
            "postgres" => Ok(Arc::new(PostgresBackend::new(config.options())?)),
            other => Err(Error::InvalidSpec(format!(
                "unknown memory backend: {other}"
            ))),
        }
    }
}

// ---------------------------------------------------------------------------
// InMemoryBackend
// ---------------------------------------------------------------------------

#[derive(Debug, Default, Clone)]
pub struct InMemoryBackend {
    inner: Arc<Mutex<HashMap<String, Session>>>,
}

impl InMemoryBackend {
    pub fn new() -> Self {
        Self::default()
    }

    fn key(domain_id: &str, session_id: &str) -> String {
        format!("{domain_id}::{session_id}")
    }
}

#[async_trait]
impl MemoryBackend for InMemoryBackend {
    async fn load_session(&self,
        domain_id: &str,
        session_id: &str,
    ) -> Result<Session> {
        let key = Self::key(domain_id, session_id);
        let map = self.inner.lock().expect("memory mutex poisoned");
        Ok(map.get(&key).cloned().unwrap_or_else(|| Session {
            domain_id: domain_id.to_string(),
            session_id: session_id.to_string(),
            turns: Vec::new(),
        }))
    }

    async fn save_turn(
        &self,
        session: &Session,
        agent_id: &str,
        role: &str,
        content: &str,
    ) -> Result<()> {
        let key = Self::key(&session.domain_id, &session.session_id);
        let mut map = self.inner.lock().expect("memory mutex poisoned");
        let entry = map.entry(key).or_insert_with(|| Session {
            domain_id: session.domain_id.clone(),
            session_id: session.session_id.clone(),
            turns: Vec::new(),
        });
        entry.turns.push(Turn {
            agent_id: agent_id.to_string(),
            role: role.to_string(),
            content: content.to_string(),
        });
        Ok(())
    }

    async fn build_context(
        &self,
        session: &Session,
        agent_id: &str,
        window: usize,
    ) -> Result<Vec<Message>> {
        let key = Self::key(&session.domain_id, &session.session_id);
        let map = self.inner.lock().expect("memory mutex poisoned");
        let Some(stored) = map.get(&key) else {
            return Ok(Vec::new());
        };
        let mut collected: Vec<&Turn> = stored
            .turns
            .iter()
            .filter(|t| t.agent_id == agent_id)
            .rev()
            .take(window)
            .collect();
        collected.reverse();
        Ok(collected
            .into_iter()
            .map(|t| Message {
                role: t.role.clone(),
                content: t.content.clone(),
            })
            .collect())
    }
}

// ---------------------------------------------------------------------------
// SqliteBackend
// ---------------------------------------------------------------------------

pub struct SqliteBackend {
    path: String,
}

impl SqliteBackend {
    pub fn new(path: impl Into<String>) -> Result<Self> {
        let path = path.into();
        let conn = rusqlite::Connection::open(&path)
            .map_err(|e| Error::Adapter(format!("SqliteBackend open: {e}")))?;
        conn.execute(
            "CREATE TABLE IF NOT EXISTS turns (
                domain_id TEXT NOT NULL,
                session_id TEXT NOT NULL,
                agent_id TEXT NOT NULL,
                role TEXT NOT NULL,
                content TEXT NOT NULL,
                ordinal INTEGER NOT NULL
            )",
            [],
        )
        .map_err(|e| Error::Adapter(format!("SqliteBackend schema: {e}")))?;
        conn.execute(
            "CREATE INDEX IF NOT EXISTS idx_turns_session ON turns(domain_id, session_id, ordinal)",
            [],
        )
        .map_err(|e| Error::Adapter(format!("SqliteBackend index: {e}")))?;
        Ok(Self { path })
    }

    fn with_conn<T, F>(&self, f: F) -> Result<T>
    where
        F: FnOnce(&rusqlite::Connection) -> Result<T>,
    {
        let conn = rusqlite::Connection::open(&self.path)
            .map_err(|e| Error::Adapter(format!("SqliteBackend open: {e}")))?;
        f(&conn)
    }
}

#[async_trait]
impl MemoryBackend for SqliteBackend {
    async fn load_session(
        &self,
        domain_id: &str,
        session_id: &str,
    ) -> Result<Session> {
        self.with_conn(|conn| {
            let mut stmt = conn.prepare(
                "SELECT agent_id, role, content FROM turns
                 WHERE domain_id = ? AND session_id = ?
                 ORDER BY ordinal"
            ).map_err(|e| Error::Adapter(format!("SqliteBackend prepare: {e}")))?;
            let rows = stmt.query_map([domain_id, session_id], |row| {
                Ok(Turn {
                    agent_id: row.get(0)?,
                    role: row.get(1)?,
                    content: row.get(2)?,
                })
            }).map_err(|e| Error::Adapter(format!("SqliteBackend query: {e}")))?;
            let mut turns = Vec::new();
            for row in rows {
                turns.push(row.map_err(|e| Error::Adapter(format!("SqliteBackend load row: {e}")))?);
            }
            Ok(Session {
                domain_id: domain_id.to_string(),
                session_id: session_id.to_string(),
                turns,
            })
        })
    }

    async fn save_turn(
        &self,
        session: &Session,
        agent_id: &str,
        role: &str,
        content: &str,
    ) -> Result<()> {
        let domain_id = session.domain_id.clone();
        let session_id = session.session_id.clone();
        let agent_id = agent_id.to_string();
        let role = role.to_string();
        let content = content.to_string();
        self.with_conn(move |conn| {
            let ordinal: i64 = conn.query_row(
                "SELECT COALESCE(MAX(ordinal), 0) + 1 FROM turns
                 WHERE domain_id = ? AND session_id = ?",
                [&domain_id as &dyn rusqlite::ToSql,
                    &session_id as &dyn rusqlite::ToSql],
                |row| row.get(0),
            ).map_err(|e| Error::Adapter(format!("SqliteBackend query_row: {e}")))?;
            conn.execute(
                "INSERT INTO turns (domain_id, session_id, agent_id, role, content, ordinal)
                 VALUES (?, ?, ?, ?, ?, ?)",
                [
                    &domain_id as &dyn rusqlite::ToSql,
                    &session_id as &dyn rusqlite::ToSql,
                    &agent_id as &dyn rusqlite::ToSql,
                    &role as &dyn rusqlite::ToSql,
                    &content as &dyn rusqlite::ToSql,
                    &ordinal as &dyn rusqlite::ToSql,
                ],
            ).map_err(|e| Error::Adapter(format!("SqliteBackend insert: {e}")))?;
            Ok(())
        })
    }

    async fn build_context(
        &self,
        session: &Session,
        agent_id: &str,
        window: usize,
    ) -> Result<Vec<Message>> {
        self.with_conn(|conn| {
            let mut stmt = conn.prepare(
                "SELECT role, content FROM turns
                 WHERE domain_id = ? AND session_id = ? AND agent_id = ?
                 ORDER BY ordinal DESC
                 LIMIT ?"
            ).map_err(|e| Error::Adapter(format!("SqliteBackend prepare: {e}")))?;
            let window = window as i64;
            let rows = stmt.query_map(
                [
                    &session.domain_id as &dyn rusqlite::ToSql,
                    &session.session_id as &dyn rusqlite::ToSql,
                    &agent_id as &dyn rusqlite::ToSql,
                    &window as &dyn rusqlite::ToSql,
                ],
                |row| {
                    Ok(Message {
                        role: row.get(0)?,
                        content: row.get(1)?,
                    })
                },
            ).map_err(|e| Error::Adapter(format!("SqliteBackend query: {e}")))?;
            let mut messages = Vec::new();
            for row in rows {
                messages.push(row.map_err(|e| Error::Adapter(format!("SqliteBackend context row: {e}")))?);
            }
            // Reverse to oldest-first.
            messages.reverse();
            Ok(messages)
        })
    }
}

// ---------------------------------------------------------------------------
// RedisBackend (placeholder)
// ---------------------------------------------------------------------------

pub struct RedisBackend {
    #[allow(dead_code)]
    url: String,
}

impl RedisBackend {
    pub fn new(options: &Value) -> Result<Self> {
        let url = options
            .get("url")
            .and_then(Value::as_str)
            .unwrap_or("redis://127.0.0.1:6379")
            .to_string();
        Ok(Self { url })
    }
}

#[async_trait]
impl MemoryBackend for RedisBackend {
    async fn load_session(
        &self,
        _domain_id: &str,
        _session_id: &str,
    ) -> Result<Session> {
        Err(Error::Unimplemented(
            "RedisBackend is a placeholder in this version",
        ))
    }

    async fn save_turn(
        &self,
        _session: &Session,
        _agent_id: &str,
        _role: &str,
        _content: &str,
    ) -> Result<()> {
        Err(Error::Unimplemented(
            "RedisBackend is a placeholder in this version",
        ))
    }

    async fn build_context(
        &self,
        _session: &Session,
        _agent_id: &str,
        _window: usize,
    ) -> Result<Vec<Message>> {
        Err(Error::Unimplemented(
            "RedisBackend is a placeholder in this version",
        ))
    }
}

// ---------------------------------------------------------------------------
// PostgresBackend (placeholder)
// ---------------------------------------------------------------------------

pub struct PostgresBackend {
    #[allow(dead_code)]
    url: String,
}

impl PostgresBackend {
    pub fn new(options: &Value) -> Result<Self> {
        let url = options
            .get("url")
            .and_then(Value::as_str)
            .unwrap_or("postgresql://postgres@localhost/hnsx")
            .to_string();
        Ok(Self { url })
    }
}

#[async_trait]
impl MemoryBackend for PostgresBackend {
    async fn load_session(
        &self,
        _domain_id: &str,
        _session_id: &str,
    ) -> Result<Session> {
        Err(Error::Unimplemented(
            "PostgresBackend is a placeholder in this version",
        ))
    }

    async fn save_turn(
        &self,
        _session: &Session,
        _agent_id: &str,
        _role: &str,
        _content: &str,
    ) -> Result<()> {
        Err(Error::Unimplemented(
            "PostgresBackend is a placeholder in this version",
        ))
    }

    async fn build_context(
        &self,
        _session: &Session,
        _agent_id: &str,
        _window: usize,
    ) -> Result<Vec<Message>> {
        Err(Error::Unimplemented(
            "PostgresBackend is a placeholder in this version",
        ))
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use futures::executor::block_on;

    fn fresh() -> InMemoryBackend {
        InMemoryBackend::new()
    }

    #[test]
    fn load_missing_session_returns_empty() {
        let mem = fresh();
        let s = block_on(mem.load_session("d1", "s1")).expect("load");
        assert_eq!(s.domain_id, "d1");
        assert_eq!(s.session_id, "s1");
        assert!(s.turns.is_empty());
    }

    #[test]
    fn save_turn_appends_in_order() {
        let mem = fresh();
        let session = block_on(mem.load_session("d1", "s1")).unwrap();
        block_on(mem.save_turn(&session, "a", "user", "hello")).unwrap();
        block_on(mem.save_turn(&session, "a", "assistant", "hi")).unwrap();
        let loaded = block_on(mem.load_session("d1", "s1")).unwrap();
        assert_eq!(loaded.turns.len(), 2);
        assert_eq!(loaded.turns[0].role, "user");
        assert_eq!(loaded.turns[1].role, "assistant");
    }

    #[test]
    fn build_context_filters_by_agent_and_respects_window() {
        let mem = fresh();
        let session = block_on(mem.load_session("d1", "s1")).unwrap();
        for i in 0..5 {
            block_on(mem.save_turn(&session, "a", "user", &format!("a-turn-{i}"))).unwrap();
        }
        for i in 0..3 {
            block_on(mem.save_turn(&session, "b", "user", &format!("b-turn-{i}"))).unwrap();
        }
        let ctx = block_on(mem.build_context(&session, "a", 2)).unwrap();
        assert_eq!(ctx.len(), 2);
        assert_eq!(ctx[0].content, "a-turn-3");
        assert_eq!(ctx[1].content, "a-turn-4");

        let ctx = block_on(mem.build_context(&session, "b", 10)).unwrap();
        assert_eq!(ctx.len(), 3);
        assert_eq!(ctx[0].content, "b-turn-0");
        assert_eq!(ctx[2].content, "b-turn-2");
    }

    #[test]
    fn sessions_are_isolated_by_id() {
        let mem = fresh();
        let s1 = block_on(mem.load_session("d1", "s1")).unwrap();
        let s2 = block_on(mem.load_session("d1", "s2")).unwrap();
        block_on(mem.save_turn(&s1, "a", "user", "session-1")).unwrap();
        block_on(mem.save_turn(&s2, "a", "user", "session-2")).unwrap();

        let ctx_s1 = block_on(mem.build_context(&s1, "a", 10)).unwrap();
        let ctx_s2 = block_on(mem.build_context(&s2, "a", 10)).unwrap();
        assert_eq!(ctx_s1.len(), 1);
        assert_eq!(ctx_s1[0].content, "session-1");
        assert_eq!(ctx_s2.len(), 1);
        assert_eq!(ctx_s2[0].content, "session-2");
    }

    #[test]
    fn build_context_on_missing_session_is_empty() {
        let mem = fresh();
        let s = Session {
            domain_id: "d1".into(),
            session_id: "ghost".into(),
            turns: Vec::new(),
        };
        let ctx = block_on(mem.build_context(&s, "a", 10)).unwrap();
        assert!(ctx.is_empty());
    }

    #[test]
    fn factory_creates_in_memory() {
        let backend = MemoryBackendFactory::create(&MemoryConfig::Short("in_memory".into()))
            .expect("create");
        let s = block_on(backend.load_session("d", "s")).expect("load");
        assert!(s.turns.is_empty());
    }

    #[test]
    fn factory_creates_sqlite() {
        let dir = tempfile::tempdir().expect("tempdir");
        let path = dir.path().join("mem.db");
        let config = MemoryConfig::Structured {
            backend: "sqlite".into(),
            options: serde_json::json!({"path": path.to_string_lossy().to_string()}),
        };
        let backend = MemoryBackendFactory::create(&config).expect("create");
        let session = block_on(backend.load_session("d1", "s1")).expect("load");
        block_on(backend.save_turn(&session, "a", "user", "hello")).expect("save");
        block_on(backend.save_turn(&session, "a", "assistant", "hi")).expect("save");

        // Re-open the same file to simulate cross-process persistence.
        let backend2 = MemoryBackendFactory::create(&config).expect("create");
        let loaded = block_on(backend2.load_session("d1", "s1")).expect("load");
        assert_eq!(loaded.turns.len(), 2);

        let ctx = block_on(backend2.build_context(&loaded, "a", 10)).expect("ctx");
        assert_eq!(ctx.len(), 2);
        assert_eq!(ctx[0].content, "hello");
        assert_eq!(ctx[1].content, "hi");
    }

    #[test]
    fn factory_rejects_unknown_backend() {
        let err = match MemoryBackendFactory::create(&MemoryConfig::Short("unknown".into()),
        ) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(
            matches!(err, Error::InvalidSpec(ref m) if m.contains("unknown memory backend")),
            "got: {err:?}"
        );
    }
}
