use anyhow::Result;
use clap::{Args, ValueEnum};

use hnsx_core::{DomainLoader, Error as CoreError};

#[derive(ValueEnum, Clone, Copy, Debug, Default)]
pub enum ValidateOutput {
    /// Human-readable plain text summary (default).
    #[default]
    Text,
    /// Machine-readable JSON with `valid`, `summary`, and `errors` keys.
    Json,
}

#[derive(Args, Debug)]
pub struct ValidateArgs {
    /// Path to the domain YAML
    #[arg(long)]
    pub domain: String,
    /// Output format for the validation report
    #[arg(long, value_enum, default_value_t = ValidateOutput::Text)]
    pub output: ValidateOutput,
}

pub fn exec(args: ValidateArgs) -> Result<()> {
    match DomainLoader::new().from_path(&args.domain) {
        Ok(domain) => report_success(&args, domain.spec()),
        Err(err) => report_failure(&args, &err),
    }
}

fn report_success(args: &ValidateArgs, spec: &hnsx_core::domain::DomainSpec) -> Result<()> {
    let agent_ids: Vec<&str> = spec.agents.iter().map(|a| a.id.as_str()).collect();
    let step_ids: Vec<&str> = spec.workflow.steps.iter().map(|s| s.id.as_str()).collect();

    match args.output {
        ValidateOutput::Text => {
            println!("✓ domain '{}' is valid", spec.id);
            println!("  version:     {}", spec.version);
            println!("  description: {}", spec.description);
            println!(
                "  agents:      {} ({})",
                spec.agents.len(),
                agent_ids.join(", ")
            );
            println!(
                "  steps:       {} (entry: {})",
                spec.workflow.steps.len(),
                spec.workflow.entry
            );
            if !step_ids.is_empty() {
                println!("  step ids:    {}", step_ids.join(", "));
            }
        }
        ValidateOutput::Json => {
            let report = serde_json::json!({
                "valid": true,
                "summary": {
                    "id": spec.id,
                    "version": spec.version,
                    "description": spec.description,
                    "agent_count": spec.agents.len(),
                    "agents": agent_ids,
                    "step_count": spec.workflow.steps.len(),
                    "entry": spec.workflow.entry,
                    "steps": step_ids,
                },
                "errors": [],
            });
            println!("{}", serde_json::to_string_pretty(&report).unwrap());
        }
    }
    Ok(())
}

fn report_failure(args: &ValidateArgs, err: &CoreError) -> Result<()> {
    let (title, errors) = classify(err);

    match args.output {
        ValidateOutput::Text => {
            eprintln!("✗ domain is invalid: {}", title);
            for (i, msg) in errors.iter().enumerate() {
                eprintln!("  {}. {}", i + 1, msg);
            }
            std::process::exit(1);
        }
        ValidateOutput::Json => {
            let report = serde_json::json!({
                "valid": false,
                "summary": null,
                "error_kind": title,
                "errors": errors,
            });
            println!("{}", serde_json::to_string_pretty(&report).unwrap());
            std::process::exit(1);
        }
    }
}

fn classify(err: &CoreError) -> (&'static str, Vec<String>) {
    match err {
        CoreError::InvalidSpec(msg) => ("invalid spec", split_errors(msg)),
        CoreError::Yaml(e) => ("yaml parse error", vec![e.to_string()]),
        CoreError::Io(e) => ("io error", vec![e.to_string()]),
        CoreError::Json(e) => ("json error", vec![e.to_string()]),
        CoreError::AgentNotFound(id) => ("agent not found", vec![id.clone()]),
        CoreError::Adapter(e) => ("adapter error", vec![e.clone()]),
        CoreError::Sandbox(e) => ("sandbox error", vec![e.clone()]),
        CoreError::Unimplemented(e) => ("not implemented", vec![(*e).to_string()]),
    }
}

fn split_errors(msg: &str) -> Vec<String> {
    msg.split("; ")
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .collect()
}
