# Changelog

All notable changes to the HnsX CLI are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [1.0.0] - 2026-07-11

The first GA release of the HnsX operator CLI. Lifecycle, discovery,
resource, governance, surface, and power commands all reach feature parity
with the roadmap at `docs/cli-roadmap.md`.

### Added

- Lifecycle: `up`, `down`, `restart`, `status`, `doctor`, `logs`, `reset`
- Discovery: `try`, `examples`, `completion`
- Local: `validate`, `run`
- Resource groups: `domain`, `session`, `trace`, `eval`
- Governance groups: `governance policy|secret|approval|audit|auth`
- Surfaces: `console`, `tui`, `update`
- Power: `power format|diff|replay|debug-bundle|plugin`
- Telemetry opt-in: `telemetry on|off|status`

### Changed

- `hnsx remote <x>` is now a **hidden alias tree** marked for removal in
  v1.1. Use the resource-oriented commands (`hnsx domain`, `hnsx session`,
  …) instead. The deprecation warning is printed on every invocation.

### Removed

- None (full backward compatibility via the hidden `remote` tree).

### LTS

- This release enters the **v1.0 LTS channel**. Security fixes backported
  until **2027-07-11**.

## [0.x] — pre-release history

The 0.x series shipped the operator surface iteratively:

- **v0.3 Lifesaver** — lifecycle + discovery commands
- **v0.4 Operator** — resource commands with table output
- **v0.5 Bridge** — `console`, `tui`, `update`
- **v0.6 Governance** — policy / secret / approval / audit / auth
- **v0.7 Power** — format / diff / replay / debug-bundle
- **v0.8 Ship** — real self-update + packaging scaffold

See git history (`feat(cli): ...` commits) for the per-phase detail.