//! Custom (OpenAI-compatible) API adapter.
//!
//! Phase 1.4 provides a native `reqwest` + SSE implementation in
//! `custom_adapter::CustomAdapter`. This module re-exports it for ergonomic
//! imports.

pub use crate::custom_adapter::CustomAdapter;
