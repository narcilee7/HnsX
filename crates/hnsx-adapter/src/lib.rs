#![allow(dead_code)]

//! HnsX adapter layer.
//!
//! Each adapter module wraps an external AI agent provider behind the
//! `hnsx_core::Adapter` trait. See Initial_Architectrue.md §4.3.

pub mod anthropic;
pub mod claude_code;
pub mod codex;
pub mod custom;
pub mod ollama;
pub mod openai;

pub use hnsx_core::{Adapter, AdapterConfig, RuntimeContext};

/// Factory that resolves a `Provider` to a concrete `Adapter` impl.
/// Real dispatch lands in Phase 1.
pub struct AdapterFactory;

impl AdapterFactory {
    pub fn new() -> Self {
        Self
    }
}

impl Default for AdapterFactory {
    fn default() -> Self {
        Self::new()
    }
}
