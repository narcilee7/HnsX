//! Secret loading for API keys and other sensitive configuration.
//!
//! Secrets are loaded from `.hnsx/secrets.yaml` (or an explicit path) and
//! merged with environment variables. CLI flags / env vars take precedence.

use std::collections::HashMap;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use serde::Deserialize;

/// Resolved secrets.
#[derive(Debug, Clone, Default)]
pub struct Secrets {
    /// Provider-level API keys: `openai`, `anthropic`, `custom`, etc.
    keys: HashMap<String, String>,
    /// Per-agent overrides: `agents.<agent_id>.<provider>`.
    agents: HashMap<String, HashMap<String, String>>,
}

/// Raw on-disk secrets format.
#[derive(Debug, Clone, Default, Deserialize)]
struct RawSecrets {
    #[serde(default, flatten)]
    keys: HashMap<String, String>,
    #[serde(default)]
    agents: HashMap<String, HashMap<String, String>>,
}

/// Load secrets from an explicit path, or fall back to `.hnsx/secrets.yaml`.
pub fn load_secrets(path: Option<&Path>) -> Result<Secrets> {
    let candidate = match path {
        Some(p) => p.to_path_buf(),
        None => PathBuf::from(".hnsx").join("secrets.yaml"),
    };

    if !candidate.exists() {
        return Ok(Secrets::default());
    }

    let file = std::fs::File::open(&candidate)
        .with_context(|| format!("failed to open secrets {}", candidate.display()))?;
    let raw: RawSecrets = serde_yaml::from_reader(file)
        .with_context(|| format!("failed to parse secrets {}", candidate.display()))?;

    Ok(Secrets {
        keys: raw.keys,
        agents: raw.agents,
    })
}

/// Resolve an API key for a provider, optionally scoped to an agent.
pub fn resolve_api_key(secrets: &Secrets, provider: &str, agent_id: &str) -> Option<String> {
    // 1. Per-agent override.
    if let Some(agent_keys) = secrets.agents.get(agent_id) {
        if let Some(key) = agent_keys.get(provider) {
            return Some(key.clone());
        }
    }
    // 2. Provider-level secret.
    secrets.keys.get(provider).cloned()
}

/// Read the standard environment variable for a provider.
pub fn api_key_env_var(provider: &str) -> Option<&'static str> {
    match provider {
        "openai" => Some("OPENAI_API_KEY"),
        "anthropic" => Some("ANTHROPIC_API_KEY"),
        "custom" => Some("CUSTOM_API_KEY"),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;

    #[test]
    fn missing_secrets_returns_empty() {
        let secrets = load_secrets(Some(Path::new("/nonexistent/secrets.yaml"))).unwrap();
        assert!(resolve_api_key(&secrets, "openai", "a").is_none());
    }

    #[test]
    fn loads_provider_and_agent_keys() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("secrets.yaml");
        let mut file = std::fs::File::create(&path).unwrap();
        file.write_all(
            b"openai: sk-openai\nanthropic: sk-anthropic\nagents:\n  reviewer:\n    openai: sk-reviewer\n",
        )
        .unwrap();

        let secrets = load_secrets(Some(&path)).unwrap();
        assert_eq!(
            resolve_api_key(&secrets, "openai", "other"),
            Some("sk-openai".to_string())
        );
        assert_eq!(
            resolve_api_key(&secrets, "openai", "reviewer"),
            Some("sk-reviewer".to_string())
        );
        assert_eq!(
            resolve_api_key(&secrets, "anthropic", "reviewer"),
            Some("sk-anthropic".to_string())
        );
    }

    #[test]
    fn env_var_mapping() {
        assert_eq!(api_key_env_var("openai"), Some("OPENAI_API_KEY"));
        assert_eq!(api_key_env_var("ollama"), None);
    }
}
