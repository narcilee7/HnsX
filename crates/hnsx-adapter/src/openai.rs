//! OpenAI API adapter.
//!
//! For Phase 1 the OpenAI provider is implemented by the multi-provider
//! `genai` client (`hnsx-adapter/src/genai.rs`). This module re-exports that
//! implementation so callers can still reference `hnsx_adapter::openai` if
//! desired, without maintaining a separate HTTP client here.

pub use crate::genai::{GenaiAgent, GenaiAgentFactory};
