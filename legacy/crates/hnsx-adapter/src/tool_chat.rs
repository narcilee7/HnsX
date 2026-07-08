//! Shared helpers for native adapter tool-call loops.

use serde_json::{Value, json};

use hnsx_core::agent::ToolKind;
use hnsx_core::tool::ToolRegistry;

/// Maximum number of assistant->tool->assistant round trips allowed.
pub const MAX_TOOL_ROUNDS: usize = 5;

/// Accumulates a single tool call while parsing streaming deltas.
#[derive(Debug, Default)]
pub struct PartialToolCall {
    pub id: String,
    pub name: String,
    pub arguments: String,
}

/// Build the OpenAI-compatible `tools` array for a registry.
pub fn tool_definitions(registry: &ToolRegistry) -> Option<Value> {
    crate::tools::openai_tool_definitions(registry)
}

/// Execute a tool looked up by name across all known tool kinds.
///
/// Errors are serialized into a JSON object so the LLM can see them and
/// potentially recover, rather than aborting the whole stream.
pub async fn execute_tool(registry: &ToolRegistry, name: &str, arguments: Value) -> Value {
    for kind in [
        ToolKind::Http,
        ToolKind::Python,
        ToolKind::Shell,
        ToolKind::Sql,
    ] {
        if let Some(tool) = registry.get(kind, name) {
            return match tool.invoke(arguments).await {
                Ok(value) => value,
                Err(e) => json!({"error": format!("{e}")}),
            };
        }
    }
    json!({"error": format!("tool `{name}` not found in registry")})
}
