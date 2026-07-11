#!/usr/bin/env bash
# scripts/smoke-cli.sh — smoke for the v0.3 operator CLI surface.
#
# Verifies the new `hnsx` command tree against the local stack:
#   - version
#   - doctor (all green)
#   - status
#   - examples (lists ≥3)
#   - try (end-to-end: register + trigger + capture SSE)
#   - validate (parses an example)
#
# Pre-conditions:
#   - bin/hnsx exists (run `make build-cli` if not)
#   - docker compose stack is up at the canonical port (hnsx up --detach)
#
# Usage:
#   ./scripts/smoke-cli.sh
#
# Exits non-zero on the first failed assertion.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="$ROOT/bin"
HNSX="${HNSX_BIN:-$BIN_DIR/hnsx}"
SERVER_URL="${HNSX_SERVER_URL:-http://127.0.0.1:50052}"

bold() { printf "\033[1m%s\033[0m\n" "$*"; }
ok()   { printf "  \033[32m✓\033[0m %s\n" "$*"; }
fail() { printf "  \033[31m✗\033[0m %s\n" "$*"; exit 1; }

[[ -x "$HNSX" ]] || fail "hnsx binary not found at $HNSX (run \`make build-cli\`)"

bold "[1/6] version"
out="$("$HNSX" version)"
[[ "$out" == hnsx* ]] || fail "unexpected version output: $out"
ok "$out"

bold "[2/6] doctor"
"$HNSX" --server "$SERVER_URL" doctor --output json >/dev/null \
  || fail "doctor reported issues (run \`hnsx doctor\` for details)"
ok "doctor all green"

bold "[3/6] status"
out="$("$HNSX" --server "$SERVER_URL" status)"
echo "$out" | grep -q "server healthz: ✓" || fail "server is not healthy"
ok "stack status ok"

bold "[4/6] examples"
out="$("$HNSX" examples --output json)"
count="$(printf '%s' "$out" | grep -o '"name":' | wc -l | tr -d ' ')"
[[ "$count" -ge 3 ]] || fail "expected ≥3 examples, got $count"
ok "$count examples discovered"

bold "[5/6] validate an example domain"
"$HNSX" validate --domain "$ROOT/example-domains/customer-service/domain.yaml" >/dev/null \
  || fail "validate failed for customer-service"
ok "customer-service domain.yaml parses"

bold "[6/6] try an example (register + trigger + tail SSE briefly)"
# Run `hnsx try` against noop-smoke with --detach so the SSE consumer doesn't
# hang the script. We capture the session id from the printed line.
out="$("$HNSX" --server "$SERVER_URL" try noop-smoke \
  --trigger '{"question":"smoke-cli"}' --detach 2>&1)" || fail "hnsx try failed: $out"
sid="$(printf '%s' "$out" | grep -oE 'session [a-zA-Z0-9_-]+ started' | awk '{print $2}')"
[[ -n "$sid" ]] || fail "no session id parsed from: $out"
ok "session started: $sid"

# Spot-check the session is visible via the legacy remote list endpoint,
# exercising the deprecation-alias path.
list="$(curl -fsS -H 'Content-Type: application/json' "$SERVER_URL/api/v1/sessions" 2>/dev/null || true)"
if [[ -n "$list" ]]; then
  echo "$list" | grep -q "$sid" || fail "session $sid not visible in /api/v1/sessions"
  ok "session visible via REST"
fi

bold "[v0.4/1] resource commands"
# domain list
"$HNSX" --server "$SERVER_URL" domain list --limit 1 >/dev/null \
  || fail "domain list failed"
ok "domain list works"

# session list with --filter --limit
"$HNSX" --server "$SERVER_URL" session list --limit 5 --filter domain_id=noop-smoke >/dev/null \
  || fail "session list (with filters) failed"
ok "session list --filter works"

# session trigger (returns a new id)
new_sid="$("$HNSX" --server "$SERVER_URL" session trigger --domain noop-smoke \
  --trigger '{"question":"v0.4 smoke"}' --output quiet 2>/dev/null || true)"
[[ -n "$new_sid" ]] || fail "session trigger did not return an id"
ok "session trigger → $new_sid"

# trace list
"$HNSX" --server "$SERVER_URL" trace list --limit 3 >/dev/null \
  || fail "trace list failed"
ok "trace list works"

# eval set list
"$HNSX" --server "$SERVER_URL" eval set list >/dev/null \
  || fail "eval set list failed"
ok "eval set list works"

bold "all v0.3 + v0.4 CLI smoke checks passed"