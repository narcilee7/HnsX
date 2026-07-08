//! OpenAI API adapter.
//!
//! Phase 1.4 provides a native `reqwest` + SSE implementation in
//! `openai_adapter::OpenAIAdapter`. This module re-exports it for ergonomic
//! imports.

pub use crate::openai_adapter::OpenAIAdapter;
