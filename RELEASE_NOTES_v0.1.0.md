# HnsX v0.1.0 — first monorepo release

> **Date**: 2026-07-12
> **Tag**: `v0.1.0`
> **Type**: First monorepo release
> **License**: Apache-2.0

This is the **first GitHub Release** of HnsX. It pins a build of every
component that was on `main` at the time of the v1.0.0 CLI GA — Control
Plane, Python runtime, React console, observability component library,
three SDKs (Go / Node / Python), proto schema, documentation site, and
ten example `DomainSpec`s.

`hnsx try customer-service` works out of the box on macOS and Linux
without any real LLM API key — the noop adapter covers the full
end-to-end flow.

---

## What's in the box

| Component | Version | What it does |
|---|---|---|
| `hnsx` CLI | **1.0.0** | Operator surface — lifecycle, discovery, resource, governance, surface, power, telemetry |
| `hnsx-server` | 0.1.0 | Control Plane (Go): REST + Connect/gRPC, Postgres-backed, JWT + APIKey auth |
| `hnsx-worker` | 0.1.0 | Runtime (Python): 9 LLM adapters, 11 tools, 4 agent modes, 5 sandbox backends |
| `hnsx-console` | 0.2.0 | React 19 UI: 19 pages, Monaco-based DomainSpec editor, observation timeline |
| `@hnsx/observability` | 0.1.0 | Shared chart/primitive components, Morandi theme |
| `@hnsx/sdk-node` | 0.1.0 | Typed client + SSE streaming |
| `hnsx` SDK (Python) | 0.1.0 | httpx client + `DomainSpecBuilder` |
| `hnsx/sdk/go` | placeholder | HTTP REST client — graduate at 0.2.0 |

---

## Install

```bash
# macOS / Linux
curl -fsSL https://github.com/narcilee7/HnsX/releases/download/v0.1.0/hnsx_$(uname -s | tr A-Z a-z)_$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/').tar.gz \
  | tar -xz -C ~/.local/bin
~/.local/bin/hnsx version
```

Or with Homebrew:

```bash
brew install narcilee7/hnsx/hnsx
```

---

## 30-second demo

```bash
hnsx up                                       # start the local stack
hnsx try customer-service                     # register + trigger + tail SSE
hnsx session list                             # see the session
hnsx session tail <id>                        # watch the trace
hnsx console                                  # open the GUI
```

The noop adapter means none of this needs a real LLM API key.

---

## Try the SDKs

**Python** (`pip install hnsx`):

```python
from hnsx import HnsXClient
client = HnsXClient(base_url="http://127.0.0.1:50052")
for s in client.sessions.list(limit=10):
    print(s.id, s.status)
```

**Node / TypeScript** (after the workspace is published):

```ts
import { HnsXClient } from "@hnsx/sdk-node";
const client = new HnsXClient({ baseUrl: "http://127.0.0.1:50052" });
const session = await client.sessions.trigger({ domainId: "customer-service", question: "hi" });
for await (const e of client.sessions.streamEvents(session.id)) console.log(e);
```

**Go**:

```go
client := hnsx.NewClient("http://127.0.0.1:50052")
sess, _ := client.Sessions.Trigger(ctx, &hnsx.TriggerRequest{
    DomainID: "customer-service", Question: "hi",
})
```

---

## Verification

Each binary tarball ships with a `.sha256` file. The combined
`checksums.txt` is at the bottom of the release assets:

```bash
curl -fsSL https://github.com/narcilee7/HnsX/releases/download/v0.1.0/checksums.txt
sha256sum -c checksums.txt
```

> **Note on signed artifacts**: cosign signatures + SPDX-JSON SBOM are
> scaffolded but not yet enabled on tag pushes (the `provenance` job in
> `.github/workflows/release.yml` is gated to `workflow_dispatch` until
> signing credentials are configured). See [`documents/provenance.md`](documents/provenance.md)
> for the enable checklist. Once enabled, verify with:
>
> ```bash
> cosign verify-blob \
>   --certificate-identity-regexp 'https://github.com/narcilee7/HnsX' \
>   --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
>   --signature hnsx_linux_amd64.tar.gz.sig \
>   hnsx_linux_amd64.tar.gz
> ```

---

## Known issues & limitations

- **Console has no unit tests.** This is the largest pre-1.0 test gap.
  We're tracking it under [issue tracker] and welcome PRs that add even
  one component-level test.
- **cosign / SBOM**: see above — opt-in until signing credentials exist.
- **`hnsx/sdk/go`**: still placeholder. The Go client types exist but
  the module is not yet wired into the workspace build. Plan for
  graduation at v0.2.0.
- **Secret encryption** uses a development key in the docker-compose
  stack. **Do not run the default compose file in any non-local
  environment** — see [SECURITY.md](SECURITY.md) § Hardening checklist.
- **Multi-tenancy** (`X-Tenant-ID` → tenant resolution) is on the
  roadmap but not implemented yet.

---

## What's next

We're running a 13-week Phase 1 sprint toward a stable PLG funnel.
The next release will be **v0.2.0** around the end of W4 (early August
2026), and will include:

- 5-minute `hnsx deploy` to HnsX Cloud
- Template Gallery v0.1 with 10 seed templates
- `hnsx eval <template>` for one-shot evaluation
- Console tests covering at least one page

---

## Contributing

PRs welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) and pick a
`good first issue`, or open a Discussion first if your idea is bigger
than a one-PR change. Security issues go to `security@hnsx.io`, not
public Issues.

---

## Acknowledgments

Built by the HnsX authors and contributors. See
[`AUTHORS.md`](AUTHORS.md) (auto-generated; populated at v1.0.0).

`#HarnessAsAService`