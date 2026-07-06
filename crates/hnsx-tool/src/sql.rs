//! SQL tool: SQLite via `rusqlite`. Each call opens a connection against
//! `path` (or `:memory:`), runs the configured query, and returns rows as
//! a JSON array. Postgres lands in Phase 4.

use std::sync::Arc;

use async_trait::async_trait;
use serde::Deserialize;
use serde_json::{json, Value};

use hnsx_core::agent::ToolKind;
use hnsx_core::error::{Error, Result};
use hnsx_core::tool::Tool;

#[derive(Debug, Clone, Deserialize)]
pub struct SqlConfig {
    /// Path to the SQLite database. Use `:memory:` for an in-memory db.
    pub path: String,
    /// SQL query to run on every invocation.
    pub query: String,
}

pub struct SqlTool {
    name: String,
    config: Value,
    path: String,
    query: String,
}

impl SqlTool {
    pub fn new(name: impl Into<String>, config: Value) -> Result<Arc<Self>> {
        let cfg: SqlConfig = serde_json::from_value(config.clone())
            .map_err(|e| Error::Adapter(format!("SqlTool config: {e}")))?;
        Ok(Arc::new(Self {
            name: name.into(),
            config,
            path: cfg.path,
            query: cfg.query,
        }))
    }
}

#[async_trait]
impl Tool for SqlTool {
    fn name(&self) -> &str {
        &self.name
    }
    fn kind(&self) -> ToolKind {
        ToolKind::Sql
    }
    fn config(&self) -> &Value {
        &self.config
    }

    async fn invoke(&self, args: Value) -> Result<Value> {
        // Optional per-call params are appended to the configured query as
        // positional bind values. For 3.4 the configured query is the
        // entire statement; per-call args are mostly a placeholder for
        // future parameterisation.
        let _ = args;

        let path = self.path.clone();
        let query = self.query.clone();
        let rows = tokio::task::spawn_blocking(move || -> Result<Vec<Value>> {
            let conn = rusqlite::Connection::open(&path)
                .map_err(|e| Error::Adapter(format!("SqlTool open: {e}")))?;
            let mut stmt = conn
                .prepare(&query)
                .map_err(|e| Error::Adapter(format!("SqlTool prepare: {e}")))?;
            let column_names: Vec<String> = stmt
                .column_names()
                .into_iter()
                .map(String::from)
                .collect();
            let mapped = stmt
                .query_map([], |row| {
                    let mut obj = serde_json::Map::new();
                    for (i, name) in column_names.iter().enumerate() {
                        let v: rusqlite::types::Value = row.get(i)?;
                        obj.insert(
                            name.clone(),
                            rusqlite_value_to_json(&v),
                        );
                    }
                    Ok(Value::Object(obj))
                })
                .map_err(|e| Error::Adapter(format!("SqlTool query: {e}")))?;
            let mut out = Vec::new();
            for r in mapped {
                out.push(r.map_err(|e| Error::Adapter(format!("SqlTool row: {e}")))?);
            }
            Ok(out)
        })
        .await
        .map_err(|e| Error::Adapter(format!("SqlTool join: {e}")))??;

        Ok(json!({ "rows": rows, "count": rows.len() }))
    }
}

fn rusqlite_value_to_json(v: &rusqlite::types::Value) -> Value {
    use rusqlite::types::Value as V;
    match v {
        V::Null => Value::Null,
        V::Integer(i) => Value::from(*i),
        V::Real(f) => serde_json::Number::from_f64(*f)
            .map(Value::Number)
            .unwrap_or(Value::Null),
        V::Text(s) => Value::String(s.clone()),
        V::Blob(b) => Value::String(format!("<blob {} bytes>", b.len())),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test(flavor = "multi_thread")]
    async fn run_select_against_memory_db() {
        // Configure a one-shot CREATE + SELECT against an in-memory db.
        // Two queries in one tool are not really the design, but for a
        // single-shot test we just use a SELECT and seed with a setup
        // script through the args (not supported in 3.4). Instead, open
        // the in-memory db, seed, and then construct a SELECT tool.
        // We do this by reusing the same path ":memory:" is per-connection,
        // so the test creates a single connection and runs the SELECT.
        //
        // Simpler: use a temp file.
        let dir = tempfile::tempdir().expect("tempdir");
        let path = dir.path().join("test.db").to_string_lossy().to_string();

        // Seed via a direct connection.
        {
            let conn = rusqlite::Connection::open(&path).expect("seed open");
            conn.execute(
                "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)",
                [],
            )
            .expect("create");
            conn.execute("INSERT INTO items (name) VALUES ('alpha')", [])
                .expect("insert 1");
            conn.execute("INSERT INTO items (name) VALUES ('beta')", [])
                .expect("insert 2");
        }

        let tool = SqlTool::new(
            "list",
            json!({"path": path, "query": "SELECT id, name FROM items ORDER BY id"}),
        )
        .expect("build");

        let out = futures::executor::block_on(tool.invoke(json!({}))).expect("invoke");
        let rows = out["rows"].as_array().expect("rows is array");
        assert_eq!(rows.len(), 2);
        assert_eq!(rows[0]["name"], "alpha");
        assert_eq!(rows[1]["name"], "beta");
    }

    #[tokio::test]
    async fn empty_result_returns_zero_count() {
        let dir = tempfile::tempdir().expect("tempdir");
        let path = dir.path().join("empty.db").to_string_lossy().to_string();
        {
            let conn = rusqlite::Connection::open(&path).expect("open");
            conn.execute("CREATE TABLE t (x INT)", []).expect("create");
        }
        let tool = SqlTool::new(
            "q",
            json!({"path": path, "query": "SELECT * FROM t"}),
        )
        .expect("build");
        let out = tool.invoke(json!({})).await.expect("invoke");
        assert_eq!(out["count"], 0);
        assert_eq!(out["rows"].as_array().unwrap().len(), 0);
    }
}
