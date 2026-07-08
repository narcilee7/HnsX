//! Anthropic API adapter.
//!
//! Phase 1.4 provides a native `reqwest` + SSE implementation in
//! `anthropic_adapter::AnthropicAdapter`. This module re-exports it for
//! ergonomic imports.

pub use crate::anthropic_adapter::AnthropicAdapter;
