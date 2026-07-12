# Pull Request

> Thanks for sending a PR. Please fill in the checklist below — it helps
> reviewers and gets your change merged faster.
> See [CONTRIBUTING.md](../../CONTRIBUTING.md) for the full contribution guide.

---

## What does this PR do?

<!-- One short paragraph. What changed and why. Reference the issue it
     closes with `Fixes #123` or `Refs #123`. -->

## Type of change

<!-- Delete the types that don't apply. -->

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing
      functionality to change)
- [ ] Refactor (no behavior change)
- [ ] Documentation only
- [ ] Build / CI / tooling change

## Affected components

<!-- Check everything that this PR touches. -->

- [ ] `proto/` (API contract)
- [ ] `hnsx-server/` (Go control plane + CLI)
- [ ] `hnsx-worker/` (Python runtime)
- [ ] `hnsx-console/` (React UI)
- [ ] `observability/` (frontend component library)
- [ ] `sdk/go`, `sdk/node`, `sdk/python`
- [ ] `example-domains/`
- [ ] `deployments/`
- [ ] `docs/`, `website/`
- [ ] `.github/` (CI, templates)
- [ ] Other: __________

## Checklist

<!-- Each unchecked box will block review. -->

- [ ] `make ci` passes locally (lint + type-check + test + worker-test)
- [ ] I added or updated tests for behavior changes
- [ ] I updated docs for user-visible changes (CLI flag, API, schema,
      proto, console page)
- [ ] I updated CHANGELOG.md under "Unreleased" if the change is
      user-visible
- [ ] I read [CLAUDE.md](../../CLAUDE.md) and followed the project
      conventions
- [ ] No secrets, no real customer data, no real LLM API keys in
      commits or fixtures
- [ ] Screenshots / GIFs attached for console or TUI changes
      (drag the image into the comment box)

## How to verify

<!-- Concrete steps a reviewer can follow to confirm the change works.
     For CLI changes, paste the exact command. For console changes,
     include before/after screenshots. -->

```bash
# e.g.
make build-cli
./bin/hnsx try customer-service
./bin/hnsx session trigger --domain customer-service --question "hi"
```

## Breaking-change notes

<!-- If you checked "Breaking change" above, explain who is affected
     and what they need to do. Include a migration example. -->

---

🤖 Generated with [Claude Code](https://claude.com/claude-code)