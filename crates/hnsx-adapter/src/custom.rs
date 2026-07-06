//! Custom (OpenAI-compatible) adapter.
//!
//! For Phase 4 the Custom provider is implemented by the multi-provider
//! `genai` client (`hnsx-adapter/src/genai.rs`). A user points at any
//! OpenAI-compatible endpoint by setting `provider: custom` and either
//! providing a fully-qualified `genai_1::<model>` model name or letting the
//! runtime prefix it automatically. `GENAI_1_ENDPOINT` then selects the
//! actual base URL. This module re-exports the genai implementation so
//! callers can reference `hnsx_adapter::custom` directly.

pub use crate::genai::{GenaiAgent, GenaiAgentFactory};
