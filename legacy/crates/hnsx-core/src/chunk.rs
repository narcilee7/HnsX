//! Streaming output chunks produced by Agents and Adapters.

use serde::{Deserialize, Serialize};
use serde_json::Value;

/// A single chunk in a streaming response.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum Chunk {
    /// A piece of text content.
    Text(String),
    /// An error message; the stream may continue or end.
    Error(String),
    /// A structured artifact (file changes, token usage, etc.).
    Artifact(Artifact),
    /// Stream is finished; carries the final workflow variable bindings.
    Done { variables: Value },
}

impl Chunk {
    pub fn text(s: impl Into<String>) -> Self {
        Chunk::Text(s.into())
    }
    pub fn error(s: impl Into<String>) -> Self {
        Chunk::Error(s.into())
    }
    pub fn done(variables: Value) -> Self {
        Chunk::Done { variables }
    }
    pub fn artifact(a: Artifact) -> Self {
        Chunk::Artifact(a)
    }
}

/// Structured artifacts emitted alongside text content.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum Artifact {
    /// Files created, modified, or deleted by the agent.
    FileChanges(Vec<FileChange>),
    /// Token usage for the most recent model call.
    TokenUsage {
        prompt: u64,
        completion: u64,
        cost_usd: f64,
    },
}

/// A single filesystem change detected inside a sandbox.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FileChange {
    pub path: String,
    pub kind: FileChangeKind,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub enum FileChangeKind {
    Created,
    Modified,
    Deleted,
}
