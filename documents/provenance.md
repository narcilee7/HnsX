# Release Provenance Setup

> How to enable **cosign signatures** + **SBOM generation** in the
> [release workflow](../.github/workflows/release.yml). Status as of
> the current release: **disabled** (see [§ Current state](#current-state)).
> The job is scaffolded and tested locally; flip the switch when signing
> credentials are ready.

---

## Current state

| Capability | Status | Triggered by |
|---|---|---|
| Cross-platform binaries (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64) | ✅ Enabled | Tag push `v*` |
| Per-binary tarball + sha256 | ✅ Enabled | Tag push `v*` |
| Combined `checksums.txt` in release body | ✅ Enabled | Tag push `v*` |
| Auto-generated release notes | ✅ Enabled | Tag push `v*` |
| CHANGELOG-extracted release notes | ✅ Enabled | Tag push `v*` |
| `workflow_dispatch` for re-publishing a tag | ✅ Enabled | Manual run |
| **cosign signatures** (`.sig` per artifact) | ⏸ Disabled | `workflow_dispatch` with `enable_provenance=true` |
| **SBOM** (SPDX-JSON) | ⏸ Disabled | Same as above |
| **SLSA provenance attestation** | ❌ Not started | — |

---

## Why it's disabled by default

cosign needs an identity to sign with. Until that identity exists, the
job would fail every time, so we keep it opt-in via
`workflow_dispatch` with an explicit input. This is the same model
LangChain, Supabase, and Helm use for first-time signing setup.

When you're ready to ship signed artifacts:

1. Pick an identity model (see below).
2. Add the secret(s) to the repository.
3. Change the `if` predicate on the `provenance` job from
   `github.event.inputs.enable_provenance == 'true'` to
   `startsWith(github.ref, 'refs/tags/v')`.
4. Run a `workflow_dispatch` test, then push a tag.

---

## Choosing a cosign identity

Three options, ordered by complexity:

### A. Keyless (Sigstore public-good) — recommended for first try

- **What**: every build gets a "short-lived" cert tied to the GitHub
  Actions OIDC token. Verifiers need only the public Rekor log.
- **Setup**: nothing — `cosign sign-blob` already uses GitHub's
  `id-token: write` permission and Fulcio.
- **Trade-off**: certs expire in ~10 minutes. Fine for "I just built
  this" verification, awkward for long-term archival.
- **Reference**: <https://docs.sigstore.dev/cosign/keyless/>

### B. Self-managed key pair

- **What**: generate a key, store the private half as
  `COSIGN_PRIVATE_KEY` repo secret, ship the public half in the repo
  (e.g. `cosign.pub`).
- **Setup**: `cosign generate-key-pair`, paste private key into repo
  secret, commit `cosign.pub`.
- **Trade-off**: you own the rotation. Compromise = revoke + re-sign.
- **Reference**: <https://docs.sigstore.dev/cosign/sign/>

### C. Keyless with an owned Fulcio + Rekor

- **What**: run your own internal Sigstore stack.
- **Setup**: weeks of work, mostly relevant for very large orgs.
- **Trade-off**: full control, full ops burden.

For Phase 1 of HnsX, **A** is the right choice. Move to **B** if
customers start asking about long-term archival.

---

## What the provenance job produces

Per artifact in the release (e.g. `hnsx_linux_amd64.tar.gz`):

| File | Meaning |
|---|---|
| `hnsx_linux_amd64.tar.gz` | The binary tarball |
| `hnsx_linux_amd64.tar.gz.sig` | cosign signature |
| `hnsx_linux_amd64.tar.gz.sha256` | SHA-256 checksum |
| `hnsx.spdx.json` | SPDX-JSON SBOM covering the whole repo at release time |
| `hnsx.spdx.json.sha256` | SBOM checksum |

Verifiers run:

```bash
cosign verify-blob \
  --certificate-identity-regexp 'https://github.com/narcilee7/HnsX' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  --signature hnsx_linux_amd64.tar.gz.sig \
  hnsx_linux_amd64.tar.gz
```

---

## Setup checklist

When you're ready to enable signed releases:

- [ ] Decide on identity model (A / B / C above).
- [ ] If model B: generate `cosign.key` / `cosign.pub` and add
      `COSIGN_PRIVATE_KEY` repo secret.
- [ ] If model B: commit `cosign.pub` to the repo root.
- [ ] Update the `provenance` job's `if:` predicate.
- [ ] Run a `workflow_dispatch` test with `enable_provenance=true`.
- [ ] Verify with `cosign verify-blob` on a downloaded artifact.
- [ ] Add a one-liner to `README.md` telling users how to verify.
- [ ] Cut a real tag and confirm everything publishes.

---

## Related

- [SECURITY.md](../SECURITY.md) — vulnerability disclosure
- [`.github/workflows/release.yml`](../.github/workflows/release.yml) — the workflow itself
- [Cosign docs](https://docs.sigstore.dev/cosign/) — what cosign actually does
- [SLSA](https://slsa.dev) — the broader framework we're aiming toward