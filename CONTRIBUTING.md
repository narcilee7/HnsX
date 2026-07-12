# Contributing to HnsX

> **First time here?** Read [the project vision](documents/vision.md) and the
> [technical overview](documents/architecture.md) before opening an issue. They
> explain *why* HnsX is built the way it is and what "Harness as a Service"
> actually means. PRs that go against those documents are usually declined
> with a redirect, not a comment thread.

HnsX is an Apache-2.0 open source project. We welcome bug reports, docs
fixes, refactors, and well-scoped feature contributions. This document
covers the practical "how to contribute" details.

---

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).
By participating you agree to its terms. Be technical, be kind, assume
good intent.

---

## What we accept vs. what we don't

| Accept gladly | Discuss first | Likely declined |
|---|---|---|
| Bug fixes with a failing test | Refactors touching > 5 files | New LLM provider adapters (open an issue first — see the matrix below) |
| Doc typos, broken links, clarity | New CLI commands or flags | Workflow UI / drag-and-drop editor (out of scope for v1) |
| Performance fixes with a benchmark | New Eval scorer types | Anything that forks the project under a non-Apache-2.0 license |
| Additional DomainSpec examples | New OTel exporters | Adding dependencies that pull in > 50 MB native code |
| Translation (zh-CN, ja, etc.) | Schema additions to `proto/` | New SaaS/cloud features (HnsX Cloud is separate work) |

**Already-supported LLM adapters**: `anthropic`, `openai`, `codex`,
`claude_code`, `ollama`, plus the test doubles `noop` and `echo`. Adding
a new commercial LLM provider is fine but please file an issue first so we
can align on the adapter contract.

---

## Repository layout

```
HnsX/
├── proto/                  API single source of truth (buf-managed)
├── hnsx-server/            Go control plane + CLI
├── hnsx-worker/            Python runtime (Harness executor)
├── hnsx-console/           React control plane UI
├── observability/          Shared frontend observability components
├── sdk/{go,node,python}/   Three SDKs that share the proto schema
├── example-domains/        Example DomainSpec YAML files
├── deployments/{local,k8s} Local docker-compose + k8s manifests
├── docs/                   ⚠ Internal — vision, know-how, weekly logs (NOT user-facing)
├── documents/              ✅ Public user docs (mirrored to website/docs/)
└── website/                Rspress docs site (GitHub Pages)
```

The monorepo is heterogeneous on purpose. See [MONOREPO.md](MONOREPO.md)
for how `pnpm workspace`, `go.work`, and the top-level `Makefile` tie it
together. Skim [CLAUDE.md](CLAUDE.md) for project conventions AI coding
assistants should follow — many of those conventions also apply to
human contributors.

---

## Development workflow

### 1. Set up your environment

Prerequisites:

- **Go** ≥ 1.25
- **Python** ≥ 3.11
- **Node** ≥ 22 and **pnpm** ≥ 9.15
- **buf** ≥ 1.47 (for proto work)
- **Postgres 16** (for smoke tests; docker compose provides this)
- **Docker** + **Docker Compose** (for local stack)

Bootstrap:

```bash
git clone https://github.com/narcilee7/HnsX
cd HnsX

# TypeScript workspace
pnpm install

# Python worker (creates hnsx-worker/.venv)
make worker-install

# Install Go protoc plugins
make proto-tools

# Verify everything builds
make build
```

### 2. Pick an issue or draft a proposal

- **Bug fix / small change**: open a PR directly. Reference the issue
  with `Fixes #123` in the description.
- **New feature**: open an issue first using the `feature_request` template.
  Wait for a maintainer to confirm before writing code. We do this so
  contributors don't burn a week on something that conflicts with the
  Phase roadmap.
- **Question / how-to**: use the `question` template on GitHub
  Discussions, **not** Issues.

### 3. Branch & commit conventions

- Branch off `main` using `<type>/<short-kebab-slug>`:
  - `feat/<slug>` — new feature
  - `fix/<slug>` — bug fix
  - `docs/<slug>` — documentation only (private — see [docs/README.md](docs/README.md))
  - `documents/<slug>` — public documentation
  - `refactor/<slug>` — no behavior change
  - `chore/<slug>` — tooling, CI, dependencies
- Commit messages use **Conventional Commits**:
  `feat(server): add X`, `fix(worker): handle Y`, `docs(readme): clarify Z`.
  The scope is one of `server`, `worker`, `console`, `observability`,
  `sdk`, `proto`, `docs`, `cli`, `release`.
- One logical change per commit. Squash trivial WIP commits before review.

### 4. Quality gates (run before pushing)

```bash
# All checks
make ci

# Or individually:
make lint            # go vet + eslint
make type-check      # tsc --noEmit across TS packages
make test            # go test + TS vitest
make worker-test     # pytest
make proto-lint      # buf lint (only if proto changed)
```

CI runs the same checks on `ubuntu-latest` and `macos-latest`, plus a
real Postgres-backed smoke (`scripts/smoke.sh`). Your PR must pass both
platforms before merge.

### 5. Tests

- **Go**: table-driven tests, `testify/assert` or stdlib only. Put tests
  next to the code as `<file>_test.go` in the same package. Aim for the
  same coverage as the package you're editing.
- **Python**: pytest, one `tests/test_<module>.py` per module under
  `hnsx-worker/tests/`. Use the existing fixtures (`mock_server.py`,
  `noop` adapter) rather than hitting real LLM APIs.
- **TypeScript**: vitest. Tests for `@hnsx/observability` live alongside
  components. The console currently has no tests — adding even a smoke
  test for one page is a high-value contribution.
- **Proto**: `buf lint` and `buf breaking --against main` (run on PRs).

### 6. Pull request checklist

Use the PR template (auto-loaded). Make sure:

- [ ] Description explains *what* and *why*, not just *how*.
- [ ] Linked issue (if any) using `Fixes #123` or `Refs #123`.
- [ ] `make ci` passes locally.
- [ ] Tests added or updated for behavior changes.
- [ ] Docs updated if the change is user-visible (CLI flag, API, schema,
      proto, console page).
- [ ] No secrets, no real customer data, no real LLM API keys in
      commits or fixtures.
- [ ] Screenshots / GIFs attached for console or TUI changes.

A maintainer will review within ~3 business days. If you don't hear back,
ping `@narcilee7` on the PR.

---

## Areas where we especially welcome help

Marked with `good-first-issue` on GitHub:

- **Console tests** — even one component-level test removes a chunk of
  the v1.0 test gap (see ROADMAP § Phase 1 / § Console).
- **OpenTelemetry GenAI semantic conventions** — help align our
  `pkg/runtime/observation.go` with the v1.36+ stable conventions.
- **Chinese / Japanese translations** of the docs site.
- **DomainSpec example domains** in `example-domains/` — anything
  realistic that other users can `hnsx try` without an API key.
- **Bug triage** — reproduce, write a failing test, leave a clear repro.

---

## Release process (for maintainers)

- Go binaries: tag `vX.Y.Z` and push — `.github/workflows/release.yml`
  builds for 4 OS/arch combos, attaches tarballs + sha256, and publishes
  the GitHub Release.
- TypeScript workspace packages: changesets via `pnpm changeset` →
  `make version` → `make release`.
- Python SDK: `cd sdk/python && python -m build && twine upload dist/*`.

Both pipelines can run in the same release; the changeset history tells
you which packages moved.

---

## Communication

| Channel | Use it for |
|---|---|
| GitHub Issues | Bug reports, feature requests, concrete proposals |
| GitHub Discussions | How-to questions, show-and-tell, design discussion |
| PR comments | Implementation details for a specific PR |
| Discord (link TBD) | Real-time chat with maintainers and other users |

**Do not** file private security issues via Discord or email — see
[SECURITY.md](SECURITY.md).

---

## License

By contributing, you agree that your contributions will be licensed under
the [Apache License 2.0](LICENSE). You also confirm that you have the
right to submit the contribution (you wrote it, your employer released it,
or it's already under a compatible license).