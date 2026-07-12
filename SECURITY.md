# Security Policy

> Thanks for taking the time to disclose a vulnerability responsibly.
> HnsX is an Apache-2.0 project maintained by a small core team; clear,
> well-written reports help us fix issues faster.

---

## Supported versions

| Version | Status | Security backport until |
|---|---|---|
| `v1.0.x` (LTS) | Active | **2027-07-11** |
| `v0.x` | End of life | None — please upgrade |
| `main` branch | Pre-release | Best effort only |

We backport security fixes to the latest minor of the current LTS line
(v1.0.x). Older minors and the 0.x series receive no further patches —
please upgrade.

---

## Reporting a vulnerability

**Please do not file public GitHub Issues or Discussions for security
problems.** Public disclosure before a fix is shipped gives attackers a
free roadmap.

Report privately by email:

```
security@hnsx.io
```

If `security@hnsx.io` is not yet operational in your DNS view, the
fallback address is the address listed on the GitHub profile of
@narcilee7.

### What to include

A good report looks like this:

1. **Summary** — one or two sentences on what the issue is.
2. **Affected component** — `hnsx-server`, `hnsx-worker`, `hnsx-console`,
   an SDK, a DomainSpec example, or the website.
3. **Affected versions** — tag, commit SHA, or `main @ <date>`.
4. **Environment** — local `docker compose`, HnsX Cloud, self-hosted k8s,
   etc. Include OS / architecture if relevant.
5. **Reproduction steps** — minimal DomainSpec, curl command, or script.
   Keep it deterministic.
6. **Impact** — what an attacker can do (RCE, secret exfiltration,
   sandbox escape, audit log tampering, denial of service, …).
7. **Suggested fix** (optional) — if you already have a patch idea, link
   a branch or paste a diff.
8. **Disclosure timeline** — whether you intend to disclose publicly,
   and on what date.

### What to expect

| Stage | Time |
|---|---|
| Acknowledgement | within **3 business days** |
| Initial triage & severity rating | within **7 business days** |
| Status update (or fix ETA) | every **14 days** until resolved |
| Coordinated disclosure | by mutual agreement, usually **90 days** after the report |

We use GitHub Security Advisories for the private CVE draft. If you
don't have a GitHub account that can be added to the draft, the report
can stay entirely over email.

---

## Severity rating

We roughly follow CVSS 3.1, but the response priority is informed by
*real-world impact*, not just the score:

| Severity | Examples | Typical fix window |
|---|---|---|
| **Critical** | Sandbox escape in `microvm` backend; arbitrary code execution via Template Gallery content; secret exfiltration via crafted DomainSpec | 24-72h |
| **High** | Policy bypass enabling unapproved Tool use; AuditLog tampering; cross-tenant data leak in Cloud | 7-14 days |
| **Medium** | Information disclosure (verbose errors, path traversal in CLI); rate-limit bypass; default credentials in shipped configs | 30 days |
| **Low** | Denial of service that requires local access; missing security headers on the website | Best effort |

Anything that compromises the **integrity of the AuditLog** is treated
as High at minimum, because AuditLog tamper-resistance is a stated
security property of the platform.

---

## Scope of this policy

In scope:

- Source code in this repository (`hnsx-server`, `hnsx-worker`,
  `hnsx-console`, `observability`, `sdk/*`, `proto/`).
- DomainSpec examples under `example-domains/` that ship in releases.
- Build, packaging, and CI pipelines (`.github/workflows/`).
- Default Docker Compose / k8s manifests under `deployments/`.

Out of scope:

- Vulnerabilities in upstream dependencies. Please report those
  upstream first; we will incorporate the fix in our next patch release.
- HnsX Cloud (managed service) — that is operated under a separate
  trust model; please still report via the same channel and we'll route
  it.
- Issues that require the attacker to already have shell on the host
  running HnsX.
- Theoretical issues without a working proof of concept. We're happy
  to discuss threat models in public Discussions, but those don't
  qualify for the security SLA above.

---

## Hardening checklist for operators

If you run HnsX yourself (Docker Compose, Kubernetes, or bare metal):

- [ ] Replace the default Postgres credentials in `deployments/local/`.
      Do not run with the dev defaults in any non-local environment.
- [ ] Replace the default secret-encryption key
      (`hnsx-local-dev-secret-key-do-not-use-in-prod`) with a
      production-grade KMS-backed key.
- [ ] Put the Control Plane behind TLS termination (ingress, reverse
      proxy, or service mesh mTLS).
- [ ] Configure an OIDC or SAML identity provider for `hnsx` auth
      instead of long-lived API keys.
- [ ] Ship AuditLog to an append-only store (S3 with Object Lock,
      WORM bucket, or an external SIEM). The local Postgres AuditLog is
      not tamper-resistant on its own.
- [ ] Enable OpenTelemetry OTLP export to your central observability
      stack; do not rely on the bundled Tempo for production audit.
- [ ] Set `HNSX_TELEMETRY=off` if you do not want runtime metrics
      leaving the host.

---

## Recognition

We maintain a [Security Hall of Fame](https://github.com/narcilee7/HnsX/security/halloffame)
in the GitHub Security tab for reporters who follow responsible
disclosure. We do not currently run a paid bug bounty program — but if
you're researching HnsX and would like to coordinate scope, email
`security@hnsx.io`.

---

## License

This security policy is part of the HnsX project and is licensed under
[Apache-2.0](LICENSE).