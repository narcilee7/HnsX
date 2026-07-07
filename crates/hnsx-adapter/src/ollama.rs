//! Ollama API adapter.
//!
//! Phase 1.4 provides a native `reqwest` + NDJSON implementation in
//! `ollama_adapter::OllamaAdapter`. This module re-exports it for ergonomic
//! imports.

pub use crate::ollama_adapter::OllamaAdapter;
