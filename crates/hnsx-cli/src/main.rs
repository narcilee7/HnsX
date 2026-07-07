#![allow(dead_code)]

//! `hnsx` CLI entrypoint. Subcommands map to the operations described in

use anyhow::Result;
use clap::{Parser, Subcommand};

mod commands;
mod deploy;
mod runtime;

#[derive(Parser, Debug)]
#[command(
    name = "hnsx",
    version,
    about = "Harness X — enterprise orchestration runtime for AI agents"
)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand, Debug)]
enum Commands {
    /// Start the HnsX control plane
    ControlPlane(commands::control_plane::ControlPlaneArgs),
    /// Run a domain locally in dev mode
    Dev(commands::dev::DevArgs),
    /// Validate a domain YAML file (also serves as a schema round-trip smoke test)
    Validate(commands::validate::ValidateArgs),
    /// Test a single agent inside a domain
    Test(commands::test::TestArgs),
    /// Build a domain artifact
    Build(commands::build::BuildArgs),
    /// Deploy a domain to a target
    Deploy(commands::deploy::DeployArgs),
    /// Run a domain once with a trigger
    Run(commands::run::RunArgs),
    /// Stream logs from a domain
    Logs(commands::logs::LogsArgs),
    /// Inspect traces
    Traces(commands::traces::TracesArgs),
    /// Inspect metrics
    Metrics(commands::metrics::MetricsArgs),
}

fn main() -> Result<()> {
    let cli = Cli::parse();
    match cli.command {
        Commands::ControlPlane(a) => commands::control_plane::exec(a),
        Commands::Dev(a) => commands::dev::exec(a),
        Commands::Validate(a) => commands::validate::exec(a),
        Commands::Test(a) => commands::test::exec(a),
        Commands::Build(a) => commands::build::exec(a),
        Commands::Deploy(a) => commands::deploy::exec(a),
        Commands::Run(a) => commands::run::exec(a),
        Commands::Logs(a) => commands::logs::exec(a),
        Commands::Traces(a) => commands::traces::exec(a),
        Commands::Metrics(a) => commands::metrics::exec(a),
    }
}
