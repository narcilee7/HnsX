//! Memory backend trait and session types.

use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use crate::error::Result;

#[async_trait]
pub trait MemoryBackend: Send + Sync {
    async fn load_session(&self, domain_id: &str, session_id: &str) -> Result<Session>;
    async fn save_turn(
        &self,
        session: &Session,
        agent_id: &str,
        role: &str,
        content: &str,
    ) -> Result<()>;
    async fn build_context(
        &self,
        session: &Session,
        agent_id: &str,
        window: usize,
    ) -> Result<Vec<Message>>;
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Session {
    pub domain_id: String,
    pub session_id: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub role: String,
    pub content: String,
}

/// Top-level memory configuration attached to a Domain.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct MemoryConfig {
    #[serde(default = "default_backend")]
    pub backend: String,
}

fn default_backend() -> String {
    "in_memory".into()
}
