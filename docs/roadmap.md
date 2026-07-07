# HnsX RoadMap

> Public-facing high-level roadmap. For the detailed implementation plan,
> gap analysis, and exit criteria, see
> [`design/ProjectManagement/V1/RoadMap.md`](../design/ProjectManagement/V1/RoadMap.md).

---

## V1 Goal

Make `hnsx run` a production-capable local runtime:

- Load a `domain.yaml`.
- Execute its Workflow DAG across real or mocked LLM providers.
- Persist session Memory and write per-step Telemetry traces.
- Pass a comprehensive hermetic end-to-end test suite.

Out of scope for V1: control plane, Web UI, Build/Deploy, SDK. These are
intentionally skeletons in the current repo and will land in V1.1 or V2.

---

## V1 Phases

### Phase 1 — Core runtime loop (in progress)

| Sub-phase | Deliverable | Status |
|-----------|-------------|--------|
| 1.1 | Workflow engine: DAG, retry, timeout, error policy, template rendering | ✅ |
| 1.2 | Memory integration: InMemory + SQLite backends wired into workflow | ✅ |
| 1.3 | `HnsXAgent` composite model: Adapter + Sandbox + Memory + Tools | ✅ |
| 1.4 | Native adapters: OpenAI, Anthropic, Ollama, Custom, Claude Code CLI, Codex | ✅ |
| 1.5 | End-to-end tests + docs | ✅ |

### Phase 2 — Tool layer

- HTTP tool: auth, retries, JSON handling.
- Shell tool: sandboxed execution with command whitelist.
- SQL tool: SQLite execution.
- Python tool: subprocess execution.
- Native tool-calling in HTTP adapters (remove `genai` fallback for tools).

### Phase 3 — Sandbox hardening

- Cross-platform `process` backend.
- Linux namespace backend.
- Docker/container backend.
- Extended `SandboxSpec` (filesystem, network, command, resource, timeout policies).

### Phase 4 — CLI & observability polish

- `hnsx validate`: full schema and type checking.
- `hnsx test --agent`: single-agent invocation test.
- `hnsx run`: stdin triggers, output formatting, config file support.
- Secret management: env vars + `.hnsx/secrets.yaml`.
- Prometheus metrics / OTLP traces.

### Phase 5 — Control plane & Web UI (experimental)

- gRPC registry / scheduler / discovery.
- Web UI: domain list, trace viewer.

### Phase 6 — Build / Deploy / SDK (V1.1 or V2)

- `hnsx build` → `.hnsx.tar`.
- `hnsx deploy --target docker`.
- Python SDK design.

---

## V1 Exit Criteria

1. `hnsx run --domain domains/customer-service/domain.yaml --trigger '{"question":"..."}'`
   succeeds with real API keys.
2. `hnsx run --adapter noop` runs a full workflow without network.
3. `cargo test --workspace` and `cargo clippy --workspace -- -D warnings` are green.
4. At least one hermetic e2e test covers trigger → workflow → adapter →
   memory/telemetry → final output.
5. README and this roadmap accurately reflect what is implemented vs. planned.
