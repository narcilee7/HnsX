//! Memory backend trait and session types.

use std::collections::HashMap;
use std::sync::{Arc, Mutex};

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
    /// Append-only history of turns, in insertion order.
    #[serde(default)]
    pub turns: Vec<Turn>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Turn {
    pub agent_id: String,
    pub role: String,
    pub content: String,
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

// ---------------------------------------------------------------------------
// InMemoryBackend
// ---------------------------------------------------------------------------

/// Default in-process memory backend. Sessions are stored in a `HashMap`
/// keyed by `(domain_id, session_id)`. Concurrent access is mediated by a
/// `std::sync::Mutex` — the critical sections are short (HashMap ops) so the
/// runtime is not blocked in practice. Production deployments that need
/// cross-process persistence should use `RedisBackend` / `PostgresBackend`
/// (lands in Phase 4).
#[derive(Debug, Default, Clone)]
pub struct InMemoryBackend {
    inner: Arc<Mutex<HashMap<String, Session>>>,
}

impl InMemoryBackend {
    pub fn new() -> Self {
        Self::default()
    }

    fn key(domain_id: &str, session_id: &str) -> String {
        format!("{domain_id}::{session_id}")
    }
}

#[async_trait]
impl MemoryBackend for InMemoryBackend {
    async fn load_session(&self, domain_id: &str, session_id: &str) -> Result<Session> {
        let key = Self::key(domain_id, session_id);
        let map = self.inner.lock().expect("memory mutex poisoned");
        Ok(map.get(&key).cloned().unwrap_or_else(|| Session {
            domain_id: domain_id.to_string(),
            session_id: session_id.to_string(),
            turns: Vec::new(),
        }))
    }

    async fn save_turn(
        &self,
        session: &Session,
        agent_id: &str,
        role: &str,
        content: &str,
    ) -> Result<()> {
        let key = Self::key(&session.domain_id, &session.session_id);
        let mut map = self.inner.lock().expect("memory mutex poisoned");
        let entry = map.entry(key).or_insert_with(|| Session {
            domain_id: session.domain_id.clone(),
            session_id: session.session_id.clone(),
            turns: Vec::new(),
        });
        entry.turns.push(Turn {
            agent_id: agent_id.to_string(),
            role: role.to_string(),
            content: content.to_string(),
        });
        Ok(())
    }

    async fn build_context(
        &self,
        session: &Session,
        agent_id: &str,
        window: usize,
    ) -> Result<Vec<Message>> {
        let key = Self::key(&session.domain_id, &session.session_id);
        let map = self.inner.lock().expect("memory mutex poisoned");
        let Some(stored) = map.get(&key) else {
            return Ok(Vec::new());
        };
        // Take the last `window` turns for this agent, returned oldest-first.
        let mut collected: Vec<&Turn> = stored
            .turns
            .iter()
            .filter(|t| t.agent_id == agent_id)
            .rev()
            .take(window)
            .collect();
        collected.reverse();
        Ok(collected
            .into_iter()
            .map(|t| Message {
                role: t.role.clone(),
                content: t.content.clone(),
            })
            .collect())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use futures::executor::block_on;

    fn fresh() -> InMemoryBackend {
        InMemoryBackend::new()
    }

    #[test]
    fn load_missing_session_returns_empty() {
        let mem = fresh();
        let s = block_on(mem.load_session("d1", "s1")).expect("load");
        assert_eq!(s.domain_id, "d1");
        assert_eq!(s.session_id, "s1");
        assert!(s.turns.is_empty());
    }

    #[test]
    fn save_turn_appends_in_order() {
        let mem = fresh();
        let session = block_on(mem.load_session("d1", "s1")).unwrap();
        block_on(mem.save_turn(&session, "a", "user", "hello")).unwrap();
        block_on(mem.save_turn(&session, "a", "assistant", "hi")).unwrap();
        let loaded = block_on(mem.load_session("d1", "s1")).unwrap();
        assert_eq!(loaded.turns.len(), 2);
        assert_eq!(loaded.turns[0].role, "user");
        assert_eq!(loaded.turns[1].role, "assistant");
    }

    #[test]
    fn build_context_filters_by_agent_and_respects_window() {
        let mem = fresh();
        let session = block_on(mem.load_session("d1", "s1")).unwrap();
        for i in 0..5 {
            block_on(mem.save_turn(&session, "a", "user", &format!("a-turn-{i}"))).unwrap();
        }
        for i in 0..3 {
            block_on(mem.save_turn(&session, "b", "user", &format!("b-turn-{i}"))).unwrap();
        }
        // Agent a: 5 turns, ask for window=2 -> last 2 in order.
        let ctx = block_on(mem.build_context(&session, "a", 2)).unwrap();
        assert_eq!(ctx.len(), 2);
        assert_eq!(ctx[0].content, "a-turn-3");
        assert_eq!(ctx[1].content, "a-turn-4");

        // Agent b: 3 turns, window=10 -> all 3.
        let ctx = block_on(mem.build_context(&session, "b", 10)).unwrap();
        assert_eq!(ctx.len(), 3);
        assert_eq!(ctx[0].content, "b-turn-0");
        assert_eq!(ctx[2].content, "b-turn-2");
    }

    #[test]
    fn sessions_are_isolated_by_id() {
        let mem = fresh();
        let s1 = block_on(mem.load_session("d1", "s1")).unwrap();
        let s2 = block_on(mem.load_session("d1", "s2")).unwrap();
        block_on(mem.save_turn(&s1, "a", "user", "session-1")).unwrap();
        block_on(mem.save_turn(&s2, "a", "user", "session-2")).unwrap();

        let ctx_s1 = block_on(mem.build_context(&s1, "a", 10)).unwrap();
        let ctx_s2 = block_on(mem.build_context(&s2, "a", 10)).unwrap();
        assert_eq!(ctx_s1.len(), 1);
        assert_eq!(ctx_s1[0].content, "session-1");
        assert_eq!(ctx_s2.len(), 1);
        assert_eq!(ctx_s2[0].content, "session-2");
    }

    #[test]
    fn build_context_on_missing_session_is_empty() {
        let mem = fresh();
        let s = Session {
            domain_id: "d1".into(),
            session_id: "ghost".into(),
            turns: Vec::new(),
        };
        let ctx = block_on(mem.build_context(&s, "a", 10)).unwrap();
        assert!(ctx.is_empty());
    }
}
