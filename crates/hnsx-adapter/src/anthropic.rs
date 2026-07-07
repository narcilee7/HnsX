//! Anthropic API adapter.
//!
//! For Phase 1 the Anthropic provider is implemented by the multi-provider
//! `genai` client (`hnsx-adapter/src/genai.rs`). This module re-exports that
//! implementation so callers can still reference `hnsx_adapter::anthropic` if
//! desired, without maintaining a separate HTTP client here.

pub use crate::genai::{GenaiAgent, GenaiAgentFactory};
