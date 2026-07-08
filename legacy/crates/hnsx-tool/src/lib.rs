#![allow(dead_code)]

//! HnsX tool layer. Tools are capabilities agents can opt into via
//! `AgentSpec.tools`. See Initial_Architectrue.md §3 Tool Layer.

pub mod http;
pub mod python;
pub mod shell;
pub mod sql;
