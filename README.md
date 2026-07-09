# HnsX — Harness as a Service

> **Don't build weaker agents. Harness stronger ones.**

HnsX is a platform for safely running the strongest available agents
(Claude Code, Codex, OpenAI, Anthropic, Ollama, …) inside enterprise
constraints — declarative Harness configurations, fine-grained observability,
budgets, audit, evaluations, and a control plane for production deployments.

This repository contains Phase 1 of HnsX: the foundational CLI + control
plane + REST/SSE API + observation pipeline + proto contracts that every
subsequent phase builds on.

> Phase 1 deliberately **does not** ship a low-code Workflow editor, a
> self-hosted model runtime, or a SaaS control plane. Those are later
> phases. See [`docs/tech_overview.md`](docs/tech_overview.md) for the
> full roadmap.

---

## Repository layout

```
hnsx/
├── docs/                            Design docs (vision, API, schema, orchestration, evaluation, observation)
├── proto/                           Protobuf source — single source of truth for the API contract
│   ├── hnsx/v1/                     .proto files (domain, control_plane, observation, runtime)
│   └── buf.{yaml,gen.yaml}          buf config — `make proto` regenerates everything
├── go/migrations/                   Postgres migrations (goose format)
├── hnsx-server/                     Go server: control plane, REST+SSE API, runtime, telemetry
│   ├── cmd/
│   │   ├── hnsx/                    Operator CLI (validate / run / version)
│   │   └── hnsx-server/             Control-plane daemon (server / version)
│   ├── internal/{config,version}    Internal helpers (config loading, build info)
│   ├── pkg/
│   │   ├── api/                     REST handlers + chi router + SSE + error envelope
│   │   ├── adapter/                 Provider adapters (Noop, Echo; Anthropic/OpenAI in Phase 2)
│   │   ├── core/domain/             DomainSpec v2 model
│   │   ├── core/loader/             YAML loader + structural validator
│   │   ├── controlplane/            gRPC control plane server (Phase 2 service impls)
│   │   ├── db/                      pgx wrapper + goose migration runner
│   │   ├── observation/             Cross-package Observation event type
│   │   ├── policy/                  Budget / permission / guardrail engine
│   │   ├── runtime/                 Runner + Executor + Broadcaster + Supervisor + Workflow
│   │   ├── telemetry/               OTel + stdout + DB sinks
│   │   └── proto/gen/go/hnsx/v1/    buf-generated Go code (DO NOT EDIT)
│   ├── go.mod
│   └── *_test.go                    Unit tests (loader, runtime, api, config, observation, adapter)
├── hnsx-console/                    React 19 + Vite + Shadcn-style UI (built by a separate stream)
├── example-domains/                 4 v2 DomainSpec YAMLs (customer-service / claude-triage / code-review / financial-analysis)
├── bin/                             Built artifacts (hnsx, hnsx-server)
├── scripts/                         build.sh / test.sh / smoke.sh
├── deployments/local/               docker-compose (Postgres + Tempo + Grafana)
├── Makefile                         Top-level targets
└── .github/workflows/ci.yml         CI: proto lint+gen / go vet+test+smoke / console type-check+build
```

---

## Quick start

```bash
# 1. Build the CLI + server.
make build

# 2. Validate the bundled domains.
./bin/hnsx validate --domain example-domains/customer-service/domain.yaml --json

# 3. Run a session directly from the CLI (no server needed).
./bin/hnsx run \
  --domain example-domains/customer-service/domain.yaml \
  --adapter  noop \
  --trigger  '{"question":"why was I billed twice?"}' \
  --json

# 4. Start the control-plane daemon (REST + SSE on 127.0.0.1:50051 by default).
HNSX_HTTP_ADDR=127.0.0.1:51001 ./bin/hnsx-server server

# 5. Trigger a session via REST and watch it stream back via SSE.
SID=$(curl -fsS -X POST :51001/api/v1/sessions \
  -H 'Content-Type: application/json' \
  -d '{"domain_id":"customer-service","trigger":{"question":"hi"}}' | jq -r .id)
curl -N :51001/api/v1/sessions/$SID/events
```

The server boots in **DB-less mode** by default. To enable Postgres-backed
session storage + automatic migrations:

```bash
docker compose -f deployments/local/docker-compose.yaml up -d postgres
HNSX_DATABASE_URL='postgres://hnsx:hnsx@127.0.0.1:5432/hnsx?sslmode=disable' \
HNSX_OTEL_EXPORTER=otlp \
HNSX_OTEL_OTLP_ENDPOINT=127.0.0.1:4317 \
./bin/hnsx-server server
```

---

## REST API surface (Phase 1)

| Method | Path                                 | Description                  |
|--------|--------------------------------------|------------------------------|
| GET    | `/healthz`                           | Liveness                     |
| GET    | `/readyz`                            | Readiness (DB ping)          |
| GET    | `/api/v1/domains`                    | List registered domains      |
| POST   | `/api/v1/domains`                    | Register a new domain        |
| GET    | `/api/v1/domains/{id}`               | Domain detail                |
| PUT    | `/api/v1/domains/{id}`               | Update domain                |
| DELETE | `/api/v1/domains/{id}`               | Delete domain                |
| POST   | `/api/v1/domains/{id}/validate`      | Validate a DomainSpec body   |
| POST   | `/api/v1/domains/{id}/run`           | Trigger a session for domain |
| GET    | `/api/v1/sessions`                   | List sessions                |
| POST   | `/api/v1/sessions`                   | Trigger a session            |
| GET    | `/api/v1/sessions/{id}`              | Session detail + summary     |
| GET    | `/api/v1/sessions/{id}/trace`        | Trace summary (Phase 1: stub)|
| GET    | `/api/v1/sessions/{id}/events`       | **SSE** live observation     |
| POST   | `/api/v1/sessions/{id}/cancel`       | Cancel a running session     |
| POST   | `/api/v1/sessions/{id}/rerun`        | Re-trigger a session         |
| GET    | `/api/v1/traces`                     | List traces                  |
| GET    | `/api/v1/traces/{traceId}`           | Trace detail                 |
| GET    | `/api/v1/audit`                      | Audit log                    |
| GET    | `/api/v1/metrics`                    | Aggregate metrics            |
| GET    | `/api/v1/runtimes`                   | Runtime workers              |
| GET    | `/api/v1/secrets`                    | Secret registry (read-mask)  |
| GET    | `/api/v1/policies`                   | Policy registry              |

OpenAPI / generated TS types land in the **`hnsx-console/`** workspace
once proto generation is wired into pnpm.

See [`docs/server-design/api-design.md`](docs/server-design/api-design.md)
for the full contract including the standard error envelope.

---

## Architecture in one picture

```
┌──────────────────────────────────────────────────────┐
│  hnsx-server                                         │
│  ┌────────────┐  ┌─────────────┐  ┌──────────────┐    │
│  │ API Layer  │  │   Runtime   │  │  Telemetry   │    │
│  │  chi + SSE │──│  Executor + │──│  StdoutSink  │    │
│  │            │  │ Broadcaster │  │ OtlpGRPCSink │    │
│  └─────┬──────┘  └─────┬───────┘  └──────┬───────┘    │
│        │               │               │            │
│        └─────┬─────────┴─────────┬─────┘            │
│              │         ┌─────────┴───────┐          │
│              │         │     DB          │          │
│              │         │   pgx + goose   │          │
│              │         └────────┬────────┘          │
└──────────────┼──────────────────┼──────────────────┘
               │                  │
        ┌──────┴──────┐    ┌──────┴───────┐
        │ hnsx (CLI)  │    │ hnsx-console │
        │   validate  │    │  (Phase 1+)  │
        │   run       │    └──────────────┘
        └─────────────┘
```

The **observation** type is shared between runtime, telemetry, and SSE so
the same JSON shape is emitted everywhere (stdout, OTLP span attributes,
DB row payload, SSE event data).

---

## Development

```bash
make proto           # buf lint + buf generate  (regenerates Go proto)
make build           # build CLI + server
make vet             # go vet
make test-go         # go test ./...
./scripts/smoke.sh   # end-to-end smoke against in-process server
```

### Proto changes

1. Edit `proto/hnsx/v1/*.proto`.
2. Run `make proto` (regenerates `proto/gen/go/hnsx/v1/`).
3. Update the API handlers in `pkg/api/` to match.
4. Update the `pkg/core/domain/` model if the changes touch DomainSpec.

### Database changes

1. Add a new `NNNN_*.up.sql` (and matching `.down.sql`) under `go/migrations/`.
2. The server applies them automatically on boot via `pkg/db.Migrate`.

### Configuration

All runtime knobs come from `internal/config` and resolve in this order:
`flag` → `HNSX_*` env vars → YAML file (`--config`) → defaults.

See [`scripts/smoke.sh`](scripts/smoke.sh) for the canonical env contract.

---

## License

See source headers. Phase 1 work-in-progress.
