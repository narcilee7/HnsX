//! HnsX error type and Result alias.

use thiserror::Error;

#[derive(Debug, Error)]
pub enum Error {
    #[error("io error: {0}")]
    Io(#[from] std::io::Error),

    #[error("yaml parse error: {0}")]
    Yaml(#[from] serde_yaml::Error),

    #[error("json error: {0}")]
    Json(#[from] serde_json::Error),

    #[error("invalid domain spec: {0}")]
    InvalidSpec(String),

    #[error("agent not found: {0}")]
    AgentNotFound(String),

    #[error("adapter error: {0}")]
    Adapter(String),

    #[error("sandbox error: {0}")]
    Sandbox(String),

    #[error("not implemented: {0}")]
    Unimplemented(&'static str),
}

pub type Result<T> = std::result::Result<T, Error>;
