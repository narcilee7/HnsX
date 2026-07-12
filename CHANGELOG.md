# Changelog

All notable changes to the HnsX project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

> **How to read this file.** The HnsX repository ships several artifacts
> that move at different cadences. The **monorepo release** is the
> number you see in the GitHub tag and the `bin/hnsx` version banner.
> Sub-projects (the `hnsx` CLI surface, the Python `hnsx-worker`, the
> `hnsx-console` UI, each SDK) keep their own internal version and are
> called out in their own section here. The CLI's own roadmap is
> tracked internally — see [`docs/cli-roadmap.md`](docs/cli-roadmap.md).

---

## [Unreleased]

### Added (repo hygiene)

- **Open-source compliance**: root `LICENSE` (Apache-2.0),
  `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, `CODEOWNERS`.
- **Issue & PR templates** under `.github/ISSUE_TEMPLATE/` and
  `.github/PULL_REQUEST_TEMPLATE.md`. Security disclosure now routes to
  `security@hnsx.io`; everything else goes through the public chooser.
- **Dependabot configuration** covering Go modules, pip, npm, Docker
  base images, and GitHub Actions. Weekly batched updates only — no
  auto-merge.
- **PR labeler** so reviewers see `server`, `worker`, `console`,
  `sdk-go`, `sdk-python`, `docs`, etc. at a glance.
- **Release provenance scaffolding** (`docs/PROVENANCE.md` +
  `release.yml` `provenance` job). Currently opt-in via
  `workflow_dispatch`; see the doc for the enable checklist.
- **Makefile release helpers**: `make release-check` runs the pre-tag
  sanity suite locally; `make release-tag TAG=vX.Y.Z` creates an
  annotated tag; `make release-go-dry-run` prints what would be
  published.
- **README hero** — badges, demo GIF placeholder, copy-paste deploy
  snippet, three-language SDK example.

### Changed

- `README.md` reworked to lead with the Hero section; old content
  preserved below.
- `release.yml` now drives off Go 1.25 (matching `hnsx-server/go.mod`),
  supports `workflow_dispatch` for re-publishing a tag, and pulls
  release notes from `CHANGELOG.md` when present.
- **Public docs split**: `documents/` is now the source of truth for
  user-facing content; `docs/` is internal only. See
  [`docs/README.md`](docs/README.md) for the split rules.
- `Makefile` documents the two parallel release pipelines (changesets
  for the TS workspace, tag-based for the Go CLI/Server binaries).

---

## [0.1.0] - 2026-07-12

The first **monorepo** release of HnsX. Pins a build of every component
that was on `main` at the time of the v1.0.0 CLI GA — Control Plane,
Python runtime, React console, observability component library, three
SDKs (Go / Node / Python), proto schema, documentation site, and ten
example `DomainSpec`s. Ships pre-built `hnsx` and `hnsx-server`
binaries for linux/amd64, linux/arm64, darwin/amd64, and darwin/arm64,
each with a sha256 checksum.

### Components in this release

| Component | Version | Highlights |
|---|---|---|
| `hnsx` CLI | **1.0.0** | Full operator surface — lifecycle, discovery, resource, governance, surface, power, telemetry (see [§ 1.0.0](#100---2026-07-11)) |
| `hnsx-server` (Go) | 0.1.0 | Control Plane: REST API, Connect/gRPC scheduler, Postgres-backed repos for 8 resources, JWT + APIKey auth |
| `hnsx-worker` (Python) | 0.1.0 | Session executor + runtime, 9 LLM adapters, 11 tools, 4 agent modes, 5 sandbox backends |
| `hnsx-console` (React 19) | 0.2.0 | 19 pages, 30+ components, Monaco-based YAML editor, observation timeline |
| `@hnsx/observability` | 0.1.0 | 7 charts + 3 primitives + 3 integrations; Morandi theme tokens |
| `@hnsx/sdk-node` | 0.1.0 | Typed client, SSE streaming, vitest + msw test setup |
| `hnsx` SDK (Python) | 0.1.0 | httpx client, `DomainSpecBuilder`, SSE streaming |
| `hnsx/sdk/go` | (placeholder) | HTTP REST client; will graduate out of placeholder at 0.2.0 |
| Proto schema | buf 1.47 | 5 `.proto` files, ~22 KB; breaking-change check on PRs |
| `example-domains` | — | 10 YAMLs: customer-service, code-review, financial-analysis, claude-triage, mcp-demo, workflow-demo, noop-smoke, react-demo, skills-demo, customer-service-memory |
| Docs site | — | Rspress site on GitHub Pages (auto-deployed by `.github/workflows/website.yml`) |

### Verify the download

```bash
curl -fsSL https://github.com/narcilee7/HnsX/releases/download/v0.1.0/hnsx_$(uname -s | tr A-Z a-z)_$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/').tar.gz \
  | tar -xz -C /tmp
/tmp/hnsx version    # should print v0.1.0 (commit ...)
```

Verify the checksum against the values in
`releases/download/v0.1.0/checksums.txt`.

---

## [1.0.0] - 2026-07-11

The first GA release of the HnsX operator CLI. Lifecycle, discovery,
resource, governance, surface, and power commands all reach feature
parity with the roadmap at `docs/cli-roadmap.md`.

### Added

- Lifecycle: `up`, `down`, `restart`, `status`, `doctor`, `logs`, `reset`
- Discovery: `try`, `examples`, `completion`
- Local: `validate`, `run`
- Resource groups: `domain`, `session`, `trace`, `eval`
- Governance groups: `governance policy|secret|approval|audit|auth`
- Surfaces: `console`, `tui`, `update`
- Power: `power format|diff/replay/debug-bundle/plugin`
- Telemetry opt-in: `telemetry on|off|status`

### Changed

- `hnsx remote <x>` is now a **hidden alias tree** marked for removal
  in v1.1. Use the resource-oriented commands (`hnsx domain`,
  `hnsx session`, …) instead. The deprecation warning is printed on
  every invocation.

### Removed

- None (full backward compatibility via the hidden `remote` tree).

### LTS

- This release enters the **v1.0 LTS channel**. Security fixes
  backported until **2027-07-11**.

---

## [0.x] — CLI pre-release history

The 0.x series shipped the operator surface iteratively:

- **v0.3 Lifesaver** — lifecycle + discovery commands
- **v0.4 Operator** — resource commands with table output
- **v0.5 Bridge** — `console`, `tui`, `update`
- **v0.6 Governance** — policy / secret / approval / audit / auth
- **v0.7 Power** — format / diff / replay / debug-bundle
- **v0.8 Ship** — real self-update + packaging scaffold

See git history (`feat(cli): ...` commits) for the per-phase detail.

---

## Versioning rules

- **Monorepo releases** (this file's top-level entries) bump the
  Go binary tag and the GitHub Release. Every component is rebuilt
  from `main` at release time, even if some sub-components have not
  changed.
- **Sub-component versions** (CLI, Worker, Console, SDKs) follow
  semver and only bump when that component's API or behavior changes.
- **Pre-1.0 monorepo releases** (`0.x.y`) may include breaking
  changes; the changelog will call them out under `### Changed`.
  Once we cut `1.0.0`, breaking changes require a major bump.

## Release cadence (target)

| Channel | Cadence | Source |
|---|---|---|
| `nightly` | Daily | `main` @ 04:00 UTC (artifact only, no GitHub Release) |
| `stable` | Every 4 weeks | Tagged `vX.Y.Z`, full smoke + sign |
| `lts` | Yearly branch | `release/lts/X.Y`, security backports only |

Until the first tagged release, this section is aspirational.