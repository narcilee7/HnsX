//! Integration tests: load every example domain under `domains/` and assert
//! the loader accepts them. The integration test sits in `crates/hnsx-core`
//! and resolves the workspace root via `CARGO_MANIFEST_DIR/../..`.

use std::path::PathBuf;

use hnsx_core::DomainLoader;

fn workspace_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(|p| p.parent())
        .expect("workspace root")
        .to_path_buf()
}

fn load(name: &str) {
    let path = workspace_root()
        .join("domains")
        .join(name)
        .join("domain.yaml");
    assert!(
        path.exists(),
        "example domain YAML missing: {}",
        path.display()
    );
    let domain = DomainLoader::new()
        .from_path(&path)
        .unwrap_or_else(|e| panic!("failed to load {}: {e}", path.display()));
    let spec = domain.spec();
    assert_eq!(spec.id, name);
    assert!(!spec.agents.is_empty(), "domain has no agents");
    assert!(!spec.workflow.steps.is_empty(), "domain has no steps");
    // Every step's agent should resolve through the spec.
    for step in &spec.workflow.steps {
        assert!(
            domain.agent_spec(&step.agent).is_some(),
            "step {} references missing agent {}",
            step.id,
            step.agent
        );
    }
    // workflow.entry should match a real step.
    assert!(
        spec.workflow
            .steps
            .iter()
            .any(|s| s.id == spec.workflow.entry),
        "workflow.entry '{}' matches no step",
        spec.workflow.entry
    );
}

#[test]
fn loads_customer_service() {
    load("customer-service");
}

#[test]
fn loads_code_review() {
    load("code-review");
}

#[test]
fn loads_financial_analysis() {
    load("financial-analysis");
}
