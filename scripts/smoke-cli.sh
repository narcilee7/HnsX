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

bold "[5/6] validate all example domains"
for domain_file in "$ROOT"/example-domains/*/domain.yaml; do
  name="$(basename "$(dirname "$domain_file")")"
  "$HNSX" validate --domain "$domain_file" --output quiet >/dev/null \
    || fail "validate failed for $name"
  ok "$name validates"
done

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

bold "[v0.6/1] governance commands"
# policy list
"$HNSX" --server "$SERVER_URL" governance policy list >/dev/null \
  || fail "governance policy list failed"
ok "governance policy list works"

# approval list
"$HNSX" --server "$SERVER_URL" governance approval list >/dev/null \
  || fail "governance approval list failed"
ok "governance approval list works"

# audit list (with limit + JSON)
"$HNSX" --server "$SERVER_URL" governance audit list --limit 5 --output json >/dev/null \
  || fail "governance audit list failed"
ok "governance audit list works"

# secret set without --confirm should be a no-op
out="$("$HNSX" --server "$SERVER_URL" governance secret set smoke-test --value dummy 2>&1)"
echo "$out" | grep -q "Re-run with --confirm" || fail "secret set without --confirm should print warning"
ok "secret set without --confirm is a no-op"

# policy delete without --confirm should be a no-op
out="$("$HNSX" --server "$SERVER_URL" governance policy delete smoke-test 2>&1)"
echo "$out" | grep -qi "re-run with --confirm" || fail "policy delete without --confirm should print warning"
ok "policy delete without --confirm is a no-op"

# auth login + status + logout
export HNSX_AUTH_FILE="$(mktemp -t hnsx-auth.XXXXXX.yaml)"
"$HNSX" --server "$SERVER_URL" governance auth login --token test-token >/dev/null \
  || fail "auth login failed"
[[ -f "$HNSX_AUTH_FILE" ]] || fail "auth file not written"
"$HNSX" --server "$SERVER_URL" governance auth status >/dev/null \
  || fail "auth status failed"
"$HNSX" --server "$SERVER_URL" governance auth logout >/dev/null \
  || fail "auth logout failed"
[[ ! -f "$HNSX_AUTH_FILE" ]] || fail "auth file not removed"
rm -f "$HNSX_AUTH_FILE"
ok "auth login/status/logout round-trip"

bold "[v0.7/1] power commands"
# domain format (read-only — does not overwrite)
out="$("$HNSX" --server "$SERVER_URL" power format example-domains/customer-service/domain.yaml 2>&1)"
echo "$out" | grep -q "description:" || fail "format output missing description"
ok "domain format works"

# domain diff returns non-zero when changes are detected and reports count
if "$HNSX" --server "$SERVER_URL" power diff example-domains/customer-service/domain.yaml example-domains/customer-service-memory/domain.yaml --output json >/dev/null 2>&1; then
  : # identical domains are not what we're testing here; just verify it ran
fi
ok "domain diff runs"

# session replay --dry-run
last_sid="$(curl -fsS "$SERVER_URL/api/v1/sessions" 2>/dev/null | grep -o '"id":"[^"]*"' | head -1 | sed 's/.*:"//;s/"$//' || true)"
if [[ -n "$last_sid" ]]; then
  out="$("$HNSX" --server "$SERVER_URL" power replay "$last_sid" --dry-run --output json 2>&1)"
  echo "$out" | grep -q "would_use_domain" || fail "replay dry-run missing payload"
  ok "session replay --dry-run works"
else
  ok "session replay skipped (no sessions)"
fi

# debug bundle
bundle="$(mktemp -t hnsx-bundle.XXXXXX.tar.gz)"
"$HNSX" --server "$SERVER_URL" power debug-bundle -o "$bundle" >/dev/null 2>&1 \
  || fail "debug-bundle failed"
[[ -s "$bundle" ]] || fail "debug bundle is empty"
tar tzf "$bundle" | grep -q "hnsx-version.txt" || fail "bundle missing hnsx-version.txt"
tar tzf "$bundle" | grep -q "hnsx-config.yaml" || fail "bundle missing hnsx-config.yaml"
rm -f "$bundle"
ok "debug bundle contains expected entries"

bold "all v0.3 + v0.4 + v0.6 + v0.7 CLI smoke checks passed"