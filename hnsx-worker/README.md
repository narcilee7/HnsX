# hnsx-worker

HnsX Python capability execution plane (V1.1+).

This package contains:

- The **worker parent process** (`hnsx_worker.worker_service`) — registers with
  the Go control plane, heartbeats, pulls sessions, spawns one subprocess per
  session, streams observations back via the `StreamChannel` bidi gRPC.
- The **session runtime** (`hnsx_worker.session_runtime`) — loaded inside each
  subprocess. Loads the DomainSpec, routes by `session.mode` (single-task /
  workflow / multi-turn), invokes the Agent Adapter, emits Observations.
- Adapters, Tools, MCP clients, Sandbox backends, Memory backends, Skill
  engine — all the capability-layer implementations that V1.0 had as Go stubs.

## Status

- **Step 1:** wire contract + scaffolding. ✅
- **Step 2:** parent process + subprocess + Noop/Echo adapters. ✅
- **Step 3:** real provider adapters (`anthropic` / `openai` / `ollama`) with
  streaming + tool use; session executor streams chunks into observations and
  supports a multi-turn loop with stub tool result (real Tool Registry lands
  in M3). ✅ (this build)
- **Step 4+:** Tool Registry / MCP / Sandbox / Policy / Skill engine — see
  `docs/project_management/2026.07.09/v1_2_roadmap.md`.

## Install

```bash
make worker-install
```

This creates `.venv/`, installs `grpcio-tools` (for proto codegen), and
installs this package in editable mode with dev extras (pytest, ruff, mypy).

## Regenerate proto stubs

```bash
make proto-py        # Python only
make proto-all       # Go + Python
```

Generated files land at `hnsx_worker/proto/gen/hnsx/v1/` and are gitignored.

## Smoke

```bash
hnsx-worker --version
hnsx-worker check-proto    # imports the gen stubs; asserts 2 services present
make worker-test           # pytest suite
```

## Layout

```
hnsx-worker/
├── pyproject.toml
├── README.md
├── hnsx_worker/
│   ├── __init__.py
│   ├── __main__.py
│   ├── version.py
│   └── proto/gen/          # gitignored, populated by `make proto-py`
│       └── hnsx/v1/        # _pb2.py / _pb2_grpc.py
└── tests/
    ├── __init__.py
    └── test_imports.py
```

## See also

- `proto/hnsx/v1/worker.proto` — wire contract (WorkerService + SchedulerService)
- `design/Tech/V1/Architecture.md` V1.1 §10 — Ray-style worker architecture
- `docs/tech_v1_1_worker_pivot.md` — user-facing overview