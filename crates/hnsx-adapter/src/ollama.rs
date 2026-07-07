//! Ollama API adapter.
//!
//! For Phase 4 the Ollama provider is implemented by the multi-provider
//! `genai` client (`hnsx-adapter/src/genai.rs`). This module re-exports that
//! implementation so callers can still reference `hnsx_adapter::ollama` if
//! desired, without maintaining a separate HTTP client here.

pub use crate::genai::{GenaiAgent, GenaiAgentFactory};
