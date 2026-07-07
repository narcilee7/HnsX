//! `Tool` trait + `ToolRegistry`.
//!
//! Phase 3 introduces the tool abstraction: capabilities an agent can
//! opt into via `AgentSpec.tools`. Tools are looked up by `(kind, name)`
//! in a registry. Real implementations live in `hnsx-tool` (HTTP, Shell,
//! SQL, Python). The LLM-facing tool-calling protocol (OpenAI function
//! calling, Anthropic tool use) lands when the genai adapter is wired
//! to surface tool descriptions in 3.6+.

use std::collections::HashMap;
use std::sync::Arc;

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::agent::ToolKind;
use crate::error::Result;

/// A capability an agent can call. Implementations live in `hnsx-tool`.
#[async_trait]
pub trait Tool: Send + Sync {
    /// Stable, human-readable name. Combined with [`ToolKind`], this is
    /// the registry key.
    fn name(&self) -> &str;
    /// The kind slot of the registry key. Must match the spec.
    fn kind(&self) -> ToolKind;
    /// Free-form configuration. For 3.x the implementation owns its own
    /// config schema (e.g. HTTP timeout, SQL connection string).
    fn config(&self) -> &Value;
    /// Invoke the tool with a JSON `args` object. Returns a JSON value
    /// (text or structured). Errors are returned as `Err` and surfaced
    /// to the agent caller.
    async fn invoke(&self, args: Value) -> Result<Value>;
}

/// Spec carried alongside the tool so the registry can be queried without
/// calling the tool.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolSpec {
    pub name: String,
    pub kind: ToolKind,
    #[serde(default)]
    pub config: Value,
}

impl ToolSpec {
    pub fn new(name: impl Into<String>, kind: ToolKind, config: Value) -> Self {
        Self {
            name: name.into(),
            kind,
            config,
        }
    }
}

/// Registry: maps `(ToolKind, name) -> Arc<dyn Tool>`.
#[derive(Default, Clone)]
pub struct ToolRegistry {
    inner: HashMap<(ToolKind, String), Arc<dyn Tool>>,
}

impl ToolRegistry {
    pub fn new() -> Self {
        Self::default()
    }

    /// Register a tool. Replaces any existing tool with the same key.
    pub fn register(&mut self, tool: Arc<dyn Tool>) {
        let key = (tool.kind(), tool.name().to_string());
        self.inner.insert(key, tool);
    }

    /// Look up a tool by `(kind, name)`.
    pub fn get(&self, kind: ToolKind, name: &str) -> Option<Arc<dyn Tool>> {
        self.inner.get(&(kind, name.to_string())).cloned()
    }

    /// Number of registered tools.
    pub fn len(&self) -> usize {
        self.inner.len()
    }

    pub fn is_empty(&self) -> bool {
        self.inner.is_empty()
    }

    /// Iterate over all registered tools.
    pub fn iter(&self) -> impl Iterator<Item = (&ToolKind, &str, &Arc<dyn Tool>)> {
        self.inner.iter().map(|((k, n), t)| (k, n.as_str(), t))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::error::Error;
    use serde_json::json;
    use std::sync::OnceLock;

    struct EchoTool;

    static ECHO_CFG: OnceLock<Value> = OnceLock::new();

    #[async_trait]
    impl Tool for EchoTool {
        fn name(&self) -> &str {
            "echo"
        }
        fn kind(&self) -> ToolKind {
            ToolKind::Http
        }
        fn config(&self) -> &Value {
            ECHO_CFG.get_or_init(|| json!({}))
        }
        async fn invoke(&self, args: Value) -> Result<Value> {
            Ok(args)
        }
    }

    struct FailTool;

    static FAIL_CFG: OnceLock<Value> = OnceLock::new();

    #[async_trait]
    impl Tool for FailTool {
        fn name(&self) -> &str {
            "fail"
        }
        fn kind(&self) -> ToolKind {
            ToolKind::Shell
        }
        fn config(&self) -> &Value {
            FAIL_CFG.get_or_init(|| json!({}))
        }
        async fn invoke(&self, _args: Value) -> Result<Value> {
            Err(Error::Adapter("intentional".into()))
        }
    }

    #[test]
    fn register_and_lookup() {
        let mut r = ToolRegistry::new();
        r.register(Arc::new(EchoTool));
        assert!(r.get(ToolKind::Http, "echo").is_some());
        assert!(r.get(ToolKind::Http, "nope").is_none());
        assert!(r.get(ToolKind::Shell, "echo").is_none());
        assert_eq!(r.len(), 1);
    }

    #[test]
    fn invoke_returns_args() {
        let out = futures::executor::block_on(EchoTool.invoke(json!({"q": "hi"}))).unwrap();
        assert_eq!(out, json!({"q": "hi"}));
    }

    #[test]
    fn invoke_propagates_errors() {
        let err = futures::executor::block_on(FailTool.invoke(json!({}))).unwrap_err();
        assert!(matches!(err, Error::Adapter(_)));
    }

    #[test]
    fn replace_under_same_key() {
        let mut r = ToolRegistry::new();
        r.register(Arc::new(EchoTool));
        r.register(Arc::new(EchoTool));
        assert_eq!(r.len(), 1);
    }
}
