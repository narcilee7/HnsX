//! CLI configuration loading and merging.
//!
//! Configuration is loaded from `.hnsx/config.yaml` (or an explicit path) and
//! merged with environment variables and CLI flags. Lower-priority sources are
//! overridden by higher-priority sources.

use std::collections::HashMap;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use serde::Deserialize;

/// Resolved CLI configuration after merging all sources.
#[derive(Debug, Clone, Default)]
pub struct CliConfig {
    /// Directory where trace JSONL files are written.
    pub trace_dir: Option<PathBuf>,
    /// Default adapter kind when `--adapter` is not provided.
    pub default_adapter: Option<String>,
    /// Per-provider defaults keyed by provider name (e.g. `openai.endpoint`).
    pub defaults: HashMap<String, serde_json::Value>,
}

/// Raw on-disk configuration format.
#[derive(Debug, Clone, Default, Deserialize)]
struct RawConfig {
    #[serde(default)]
    trace_dir: Option<PathBuf>,
    #[serde(default)]
    default_adapter: Option<String>,
    #[serde(default)]
    defaults: HashMap<String, serde_json::Value>,
}

/// Load configuration from an explicit path, or fall back to `.hnsx/config.yaml`
/// in the current directory.
pub fn load_config(path: Option<&Path>) -> Result<CliConfig> {
    let candidate = match path {
        Some(p) => p.to_path_buf(),
        None => PathBuf::from(".hnsx").join("config.yaml"),
    };

    if !candidate.exists() {
        return Ok(CliConfig::default());
    }

    let file = std::fs::File::open(&candidate)
        .with_context(|| format!("failed to open config {}", candidate.display()))?;
    let raw: RawConfig = serde_yaml::from_reader(file)
        .with_context(|| format!("failed to parse config {}", candidate.display()))?;

    Ok(CliConfig {
        trace_dir: raw.trace_dir,
        default_adapter: raw.default_adapter,
        defaults: raw.defaults,
    })
}

/// Resolve a value from config defaults using a dot-separated key.
pub fn resolve_default<'a>(config: &'a CliConfig, key: &'a str) -> Option<&'a serde_json::Value> {
    config.defaults.get(key)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;

    #[test]
    fn missing_config_returns_defaults() {
        let config = load_config(Some(Path::new("/nonexistent/path.yaml"))).unwrap();
        assert!(config.trace_dir.is_none());
        assert!(config.default_adapter.is_none());
    }

    #[test]
    fn loads_valid_config() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("config.yaml");
        let mut file = std::fs::File::create(&path).unwrap();
        file.write_all(
            b"trace_dir: /tmp/traces\ndefault_adapter: hnsx\ndefaults:\n  openai:\n    endpoint: http://localhost:8080\n",
        )
        .unwrap();

        let config = load_config(Some(&path)).unwrap();
        assert_eq!(config.trace_dir, Some(PathBuf::from("/tmp/traces")));
        assert_eq!(config.default_adapter, Some("hnsx".to_string()));
        assert_eq!(
            resolve_default(&config, "openai"),
            Some(&serde_json::json!({"endpoint": "http://localhost:8080"}))
        );
    }
}
