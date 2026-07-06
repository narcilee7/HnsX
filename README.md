# HnsX

> Harness X — enterprise orchestration runtime for AI coding agents.

HnsX wraps strong-individual agents (Claude Code, Codex, GPT, Ollama, ...) and
lets you define business **Domains** in YAML. The runtime loads a Domain,
instantiates its agents under the right Sandbox policy, runs the Workflow
DAG, and streams results through Memory + Telemetry layers.

See [`design/Tech/V1/Initial_Architectrue.md`](design/Tech/V1/Initial_Architectrue.md)
for the full architecture.

## Workspace layout

```
crates/
  hnsx-core/           # runtime + traits (Domain, Agent, Adapter, Sandbox, Memory)
  hnsx-sandbox/        # sandbox implementations (Linux-only; no-op elsewhere)
  hnsx-adapter/        # OpenAI, Anthropic, Claude Code CLI, Codex, Ollama, custom
  hnsx-tool/           # HTTP, Python, SQL, Shell
  hnsx-cli/            # `hnsx` binary (clap)
  hnsx-control-plane/  # registry, scheduler, discovery, telemetry
domains/               # example domain YAMLs
docs/                  # public docs (skeleton)
tests/                 # integration tests (skeleton)
scripts/               # bootstrap/dev scripts (skeleton)
```

## Build

```bash
cargo check --workspace
cargo build -p hnsx-cli
./target/debug/hnsx --help
./target/debug/hnsx validate --domain domains/customer-service/domain.yaml
```

## Status

This is the v0.1.0 skeleton. Trait surfaces are in place but most bodies
return `Error::Unimplemented`. See `Initial_Architectrue.md` §10 for the
phased roadmap.
