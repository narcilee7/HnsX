//! Adapter-side tool wiring.
//!
//! - Builds a `ToolRegistry` from the `tools:` section of an `AgentSpec`.
//! - Converts registered tools into `genai::chat::Tool` definitions so the
//!   multi-provider LLM client can expose them to models.

use std::sync::Arc;

use genai::chat::Tool as GenaiTool;
use serde_json::{json, Value};

use hnsx_core::agent::{ToolKind, ToolRef};
use hnsx_core::error::Result;
use hnsx_core::tool::{Tool, ToolRegistry};
use hnsx_tool::http::HttpTool;
use hnsx_tool::python::PythonTool;
use hnsx_tool::shell::ShellTool;
use hnsx_tool::sql::SqlTool;

/// Construct a registry from the tool refs declared in an `AgentSpec`.
pub fn build_tool_registry(tool_refs: &[ToolRef]) -> Result<ToolRegistry> {
    let mut registry = ToolRegistry::new();
    for tool_ref in tool_refs {
        let tool: Arc<dyn Tool> = match tool_ref.kind {
            ToolKind::Http => HttpTool::new(&tool_ref.name, tool_ref.config.clone())
                .map(|t| t as Arc<dyn Tool>),
            ToolKind::Python => PythonTool::new(&tool_ref.name, tool_ref.config.clone())
                .map(|t| t as Arc<dyn Tool>),
            ToolKind::Shell => ShellTool::new(&tool_ref.name, tool_ref.config.clone())
                .map(|t| t as Arc<dyn Tool>),
            ToolKind::Sql => SqlTool::new(&tool_ref.name, tool_ref.config.clone())
                .map(|t| t as Arc<dyn Tool>),
        }?;
        registry.register(tool);
    }
    Ok(registry)
}

/// Convert one of our tools into a `genai` function-calling definition.
///
/// The tool's config may provide `description` and `schema` directly. If not,
/// a default schema for the tool kind is used.
pub fn to_genai_tool(tool: &dyn Tool) -> Option<GenaiTool> {
    let name = tool.name();
    let config = tool.config();

    let description = config
        .get("description")
        .and_then(Value::as_str)
        .map(String::from)
        .unwrap_or_else(|| default_description(tool.kind()));

    let schema = config
        .get("schema")
        .cloned()
        .unwrap_or_else(|| default_schema(tool.kind()));

    // genai only exposes string-keyed custom tools today.
    Some(
        GenaiTool::new(name)
            .with_description(description)
            .with_schema(schema),
    )
}

fn default_description(kind: ToolKind) -> String {
    match kind {
        ToolKind::Http => "Perform an HTTP request (GET/POST/PUT/DELETE)".into(),
        ToolKind::Python => "Run a Python script with JSON args passed on stdin".into(),
        ToolKind::Shell => "Run a whitelisted shell command".into(),
        ToolKind::Sql => "Execute a configured SQL query against a SQLite database".into(),
    }
}

fn default_schema(kind: ToolKind) -> Value {
    match kind {
        ToolKind::Http => json!({
            "type": "object",
            "properties": {
                "method": {
                    "type": "string",
                    "enum": ["GET", "POST", "PUT", "DELETE", "PATCH"],
                    "description": "HTTP method"
                },
                "path": {
                    "type": "string",
                    "description": "Path appended to the configured base_url"
                },
                "url": {
                    "type": "string",
                    "description": "Full URL; overrides base_url + path"
                },
                "body": {
                    "description": "JSON body for POST/PUT/PATCH"
                },
                "token": {
                    "type": "string",
                    "description": "Bearer token; overrides the configured default"
                }
            }
        }),
        ToolKind::Shell => json!({
            "type": "object",
            "properties": {
                "cmd": {
                    "type": "string",
                    "description": "Command basename (must be in the whitelist)"
                },
                "args": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Command arguments"
                }
            },
            "required": ["cmd"]
        }),
        ToolKind::Python => json!({
            "type": "object",
            "properties": {
                "args": {
                    "type": "string",
                    "description": "JSON object passed to the script on stdin"
                }
            }
        }),
        ToolKind::Sql => json!({
            "type": "object",
            "properties": {},
            "description": "The configured query is executed with no per-call parameters in this version"
        }),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use hnsx_core::error::Error;
    use serde_json::json;

    #[test]
    fn builds_registry_from_refs() {
        let refs = vec![
            ToolRef {
                kind: ToolKind::Http,
                name: "ping".into(),
                config: json!({"base_url": "http://localhost"}),
            },
            ToolRef {
                kind: ToolKind::Shell,
                name: "git".into(),
                config: json!({"allow": ["git"]}),
            },
        ];
        let registry = build_tool_registry(&refs).expect("build");
        assert_eq!(registry.len(), 2);
        assert!(registry.get(ToolKind::Http, "ping").is_some());
        assert!(registry.get(ToolKind::Shell, "git").is_some());
    }

    #[test]
    fn bad_config_returns_error() {
        let refs = vec![ToolRef {
            kind: ToolKind::Shell,
            name: "bad".into(),
            config: json!({"allow": []}),
        }];
        let err = match build_tool_registry(&refs) {
            Ok(_) => panic!("expected error"),
            Err(e) => e,
        };
        assert!(matches!(err, Error::Adapter(_)), "got: {err:?}");
    }

    #[test]
    fn genai_tool_uses_config_description_and_schema() {
        let tool = HttpTool::new(
            "api",
            json!({
                "base_url": "http://example.com",
                "description": "Call the internal API",
                "schema": {"type": "object", "properties": {"foo": {"type": "string"}}}
            }),
        )
        .expect("build");

        let genai = to_genai_tool(tool.as_ref()).expect("convert");
        assert_eq!(genai.name.as_str(), "api");
        assert_eq!(genai.description.as_deref(), Some("Call the internal API"));
        assert_eq!(
            genai.schema,
            Some(json!({"type": "object", "properties": {"foo": {"type": "string"}}}))
        );
    }

    #[test]
    fn genai_tool_falls_back_to_default_schema() {
        let tool = HttpTool::new("ping", json!({"base_url": "http://example.com"})).expect("build");
        let genai = to_genai_tool(tool.as_ref()).expect("convert");
        assert_eq!(genai.name.as_str(), "ping");
        assert!(genai.schema.as_ref().unwrap()["properties"].get("method").is_some());
    }
}
