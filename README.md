# HnsX

> Harness X — enterprise orchestration runtime for AI coding agents.

HnsX wraps strong-individual agents (Claude Code, Codex, GPT, Ollama, ...) and
lets you define business **Domains** in YAML. The runtime loads a Domain,
instantiates its agents under the right Sandbox policy, runs the Workflow DAG,
and streams results through Memory + Telemetry layers.

A Domain can run locally (`hnsx run` / `hnsx dev`), be packaged into a portable
`.hnsx.tar` artifact (`hnsx build`), and deployed as a Docker container that
registers with the HnsX control plane (`hnsx deploy`).

See [`design/ProjectManagement/V1/RoadMap.md`](design/ProjectManagement/V1/RoadMap.md) for the phased roadmap and
[`design/Tech/V1/Initial_Architectrue.md`](design/Tech/V1/Initial_Architectrue.md)
for the full architecture.

## Workspace layout

```
crates/
  hnsx-proto/          # generated gRPC types shared across crates
  hnsx-core/           # runtime + traits (Domain, Agent, Adapter, Sandbox, Memory, Telemetry, Package)
  hnsx-sandbox/        # sandbox implementations (process + Linux namespace backends)
  hnsx-adapter/        # OpenAI, Anthropic, Claude Code CLI, Codex, Ollama, custom
  hnsx-tool/           # HTTP, Python, SQL, Shell
  hnsx-cli/            # `hnsx` binary (clap)
  hnsx-control-plane/  # registry, scheduler, discovery, telemetry aggregation, REST API, Web UI static host
domains/               # example domain YAMLs
web/                   # React + Vite console (domains, instances, traces, metrics)
docs/                  # public docs
```

## Build

```bash
cargo check --workspace
cargo build --release --bin hnsx
./target/release/hnsx --help
```

## Quick start

Validate an example domain:

```bash
./target/release/hnsx validate --domain domains/customer-service/domain.yaml
```

Run a domain once locally (uses the HnsX factory; set `OPENAI_API_KEY` or
Ollama when using real providers, or use `--adapter noop` for CI):

```bash
./target/release/hnsx run \
  --domain domains/customer-service/domain.yaml \
  --trigger '{"question":"What is the status of my order?"}'
```

Start the control plane (gRPC on 50051, HTTP + Web UI on 50052):

```bash
./target/release/hnsx control-plane --addr 127.0.0.1:50051 --static-dir web/dist
```

Package and deploy a domain as a Docker container:

```bash
./target/release/hnsx build \
  --domain domains/financial-analysis/domain.yaml \
  --output /tmp/fa.hnsx.tar
./target/release/hnsx deploy --artifact /tmp/fa.hnsx.tar --target docker
```

Inspect telemetry:

```bash
./target/release/hnsx traces --domain-id financial-analysis
./target/release/hnsx metrics --domain-id financial-analysis
```

Browse the Web UI at `http://127.0.0.1:50052` when the control plane is running
with `--static-dir web/dist`. For local UI development use `pnpm dev` in `web/`
(Vite proxies `/api` and `/metrics` to the control plane HTTP port).

## Status

The project is currently focused on **Phase 1: core runtime**. Earlier phases
are partial or skeletal; control plane, Web UI, Build/Deploy, and SDK work is
planned for later phases. See [`design/ProjectManagement/V1/RoadMap.md`](design/ProjectManagement/V1/RoadMap.md)
for the detailed, honest roadmap.

| Phase | Milestone | Status |
|---|---|---|
| 0 | Skeleton (6 crates + CLI) | ✅ |
| 1 | Local end-to-end runtime | 🚧 core loops work; Adapter realignment done |
| 2 | Cross-platform sandbox + Claude Code CLI | 🚧 CLI adapters realigned; real sandbox backends pending |
| 3 | Tool layer (HTTP / Shell / SQL / Python) | 🚧 HTTP tool works; end-to-end tool calling pending |
| 4 | Remaining adapters + multi-backend Memory | ✅ Memory done; native HTTP/CLI adapters done |
| 5 | Control-plane gRPC (Registry / Scheduler / Discovery / Telemetry) | 🚧 skeleton only |
| 6 | Observability + Web UI | 🚧 local JSONL traces only |
| 7 | Build & Deploy (package + Docker) | 🚧 skeleton only |
| 8 | Python SDK | ⏸ planned, not started |
| 9 | Ecosystem (registry, TS SDK, more adapters) | v2.0 |

## Test

```bash
cargo test --workspace
cargo clippy --workspace -- -D warnings
```

## License

Apache-2.0
