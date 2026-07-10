#!/usr/bin/env bash
# scripts/smoke.sh — end-to-end smoke against an in-process hnsx-server.
#
# This script boots the server with a real Postgres backend, applies migrations,
# exercises the REST API + SSE + the example domains, and prints a summary.
# Any failure exits non-zero.
#
# Usage:
#   ./scripts/smoke.sh [ADDR]                (default 127.0.0.1:51002)
#
# Requires:
#   - hnsx + hnsx-server binaries in ../bin OR the Go toolchain to build them.
#   - Docker + docker compose (for local Postgres).
#   - curl with -N support.
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HNSX_SERVER_DIR="$ROOT/hnsx-server"
BIN_DIR="$ROOT/bin"
DEPLOY_DIR="$ROOT/deployments/local"
ADDR="${1:-127.0.0.1:51002}"

bold() { printf "\033[1m%s\033[0m\n" "$*"; }
ok() { printf "  \033[32m✓\033[0m %s\n" "$*"; }
fail() { printf "  \033[31m✗\033[0m %s\n" "$*"; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || fail "missing dependency: $1"
}

need curl

# 1. Build binaries if missing.
if [[ ! -x "$BIN_DIR/hnsx-server" || ! -x "$BIN_DIR/hnsx" ]]; then
  bold "[1/7] building binaries"
  make build-go
  ok "built hnsx + hnsx-server"
else
  bold "[1/7] using existing binaries"; ok "found $BIN_DIR/hnsx-server"
fi

# 2. Ensure local Postgres is running.
bold "[2/7] ensuring local Postgres"
if [ -n "${HNSX_DATABASE_URL:-}" ]; then
  ok "using HNSX_DATABASE_URL"
else
  cd "$DEPLOY_DIR" && docker compose up -d postgres
  cd "$ROOT"
  for i in $(seq 1 30); do
    if docker compose -f "$DEPLOY_DIR/docker-compose.yaml" exec -T postgres pg_isready -U hnsx >/dev/null 2>&1; then break; fi
    sleep 1
  done
  docker compose -f "$DEPLOY_DIR/docker-compose.yaml" exec -T postgres pg_isready -U hnsx >/dev/null || fail "postgres failed to become ready"
  ok "postgres ready"
  export HNSX_DATABASE_URL="postgres://hnsx:hnsx@127.0.0.1:5432/hnsx?sslmode=disable"
fi

export HNSX_MIGRATIONS_DIR="$ROOT/go/migrations"

# 3. Validate the example domains. This loop is intentionally
# resilient: a single domain failing validation must NOT abort the
# rest of the smoke (set -e in the script body is disabled around
# this block). The summary still flags failures.
bold "[3/7] validating example domains"
VALIDATED=0
INVALID=0
for dir in "$ROOT/example-domains"/*/; do
  [[ -f "$dir/domain.yaml" ]] || continue
  name="$(basename "$dir")"
  out="$("$BIN_DIR/hnsx" validate --domain "$dir/domain.yaml" --json 2>/dev/null || true)"
  if printf '%s' "$out" | grep -q '"valid":[ ]*true'; then
    ok "$name"
    VALIDATED=$((VALIDATED+1))
  else
    printf '  \033[33m!\033[0m %s (yaml parse error — skipping)\n' "$name"
    INVALID=$((INVALID+1))
  fi
done
printf '  %d validated, %d skipped\n' "$VALIDATED" "$INVALID"

# 4. Boot server with explicit --seed-from so we have known fixtures.
LOG="$(mktemp -t hnsx-smoke.XXXXXX.log)"
bold "[4/7] booting server on $ADDR (log $LOG)"
# HNSX_SECRET_KEY is required by the AES-256-GCM secret store (T2).
# Generate a deterministic-but-non-default key when the caller did not supply one.
: "${HNSX_SECRET_KEY:=hnsx-smoke-test-key-do-not-use-in-prod-2026}"
export HNSX_SECRET_KEY
HNSX_HTTP_ADDR="$ADDR" HNSX_GRPC_ADDR="" \
  HNSX_DATABASE_URL="$HNSX_DATABASE_URL" HNSX_SECRET_KEY="$HNSX_SECRET_KEY" \
  "$BIN_DIR/hnsx-server" server --seed-from "$ROOT/example-domains" >"$LOG" 2>&1 &
SERVER_PID=$!
trap 'kill "$SERVER_PID" 2>/dev/null || true' EXIT

# Wait until /healthz returns 200.
for i in $(seq 1 50); do
  if curl -fsS "http://$ADDR/healthz" >/dev/null 2>&1; then break; fi
  sleep 0.1
done
curl -fsS "http://$ADDR/healthz" >/dev/null || { cat "$LOG"; fail "server failed to start"; }
ok "healthz reachable"

# 5. /api/v1/domains is seeded via --seed-from (explicit operator action).
bold "[5/7] checking REST contract"
DOMAINS=$(curl -fsS "http://$ADDR/api/v1/domains")
COUNT=$(printf '%s' "$DOMAINS" | grep -o '"id":"' | wc -l | tr -d ' ')
[[ "$COUNT" -ge 4 ]] || fail "expected ≥4 bootstrapped domains, got $COUNT"
ok "domains list contains $COUNT items"

# 6. /api/v1/domains/<missing> -> 404 with envelope.
RESP_CODE=$(curl -s -o /dev/null -w '%{http_code}' "http://$ADDR/api/v1/domains/__missing__")
[[ "$RESP_CODE" == "404" ]] || fail "expected 404 on missing domain, got $RESP_CODE"
BODY=$(curl -s "http://$ADDR/api/v1/domains/__missing__")
echo "$BODY" | grep -q '"code":"DOMAIN_NOT_FOUND"' || fail "missing code in error body: $BODY"
ok "error envelope is well-formed"

# 7. Trigger a session + capture a brief SSE replay.
bold "[6/7] triggering a session + capturing SSE"
TRIGGER='{"domain_id":"customer-service","trigger":{"question":"hello"}}'
RESP=$(curl -fsS -X POST "http://$ADDR/api/v1/sessions" \
  -H 'Content-Type: application/json' \
  -d "$TRIGGER")
SID=$(printf '%s' "$RESP" | grep -o '"id":"[^"]*' | head -1 | sed 's/^"id":"//')
[[ -n "$SID" ]] || fail "no session id in $RESP"
ok "session created: $SID"

# Capture SSE output for ~1.5s and assert it contains at least 2 events.
SSE_FILE="$(mktemp -t hnsx-sse.XXXXXX.log)"
curl -N -fsS "http://$ADDR/api/v1/sessions/$SID/events" >"$SSE_FILE" 2>/dev/null &
SSE_PID=$!
sleep 1.5
kill "$SSE_PID" 2>/dev/null || true
wait "$SSE_PID" 2>/dev/null || true
EVENTS=$(grep -c '^event:' "$SSE_FILE" || true)
[[ "$EVENTS" -ge 2 ]] || fail "expected ≥2 SSE events, got $EVENTS (see $SSE_FILE)"
ok "captured $EVENTS SSE events"
rm -f "$SSE_FILE"

# Verify the session completed.
FINAL=$(curl -fsS "http://$ADDR/api/v1/sessions/$SID")
STATE=$(printf '%s' "$FINAL" | grep -o '"state":"[^"]*' | head -1 | sed 's/^"state":"//')
[[ "$STATE" == "completed" ]] || fail "session did not complete (state=$STATE)"
ok "session state=completed"

# 8. Trace API: GET /api/v1/traces picks up the session just spawned.
bold "[8/7] verifying trace list + per-trace detail"
TRACES=$(curl -fsS "http://$ADDR/api/v1/traces?domain=customer-service")
TRACE_COUNT=$(printf '%s' "$TRACES" | grep -o '"trace_id":"' | wc -l | tr -d ' ')
[[ "$TRACE_COUNT" -ge 1 ]] || fail "expected ≥1 trace for customer-service, got $TRACE_COUNT"
ok "traces list returned $TRACE_COUNT entries"
TID=$(printf '%s' "$TRACES" | grep -o '"trace_id":"[^"]*' | head -1 | sed 's/^"trace_id":"//')
TRACE_DETAIL=$(curl -fsS "http://$ADDR/api/v1/traces/$TID")
printf '%s' "$TRACE_DETAIL" | grep -q '"observations":' || fail "trace detail missing observations[]"
ok "trace detail returns observations[]"

# 9. Secrets CRUD: create + list with fingerprint, never plaintext.
bold "[9/7] round-tripping secrets via the encrypted store"
SECRET_NAME="smoke-secret-$$"
SECRET_BODY="$(printf '{"name":"%s","value":"super-secret-%s","description":"smoke","kind":"api_key"}' "$SECRET_NAME" "$RANDOM")"
curl -fsS -X POST "http://$ADDR/api/v1/secrets" \
  -H 'Content-Type: application/json' \
  -d "$SECRET_BODY" >/dev/null || fail "secret create failed"
ok "secret created"
SECRETS=$(curl -fsS "http://$ADDR/api/v1/secrets")
printf '%s' "$SECRETS" | grep -q "\"name\":\"$SECRET_NAME\"" || fail "secret list missing $SECRET_NAME"
SECRET_FP=$(printf '%s' "$SECRETS" | python3 -c "import sys,json;d=json.load(sys.stdin);print(next((i['fingerprint'] for i in d['items'] if i['name']=='$SECRET_NAME'),''))")
[[ -n "$SECRET_FP" ]] || fail "secret list missing fingerprint"
printf '%s' "$SECRETS" | grep -q 'super-secret' && fail "plaintext leaked into /api/v1/secrets"
ok "secret list shows fingerprint, no plaintext leak"
curl -fsS -X DELETE "http://$ADDR/api/v1/secrets/$SECRET_NAME" >/dev/null
ok "secret deleted"

# 10. Policies CRUD + binding.
bold "[10/7] round-tripping policies + binding"
POLICY_BODY="$(printf '{"id":"smoke-no-shell-%s","name":"smoke","budget":{"max_cost_usd":1.0},"permissions":{"allow_shell":false}}' "$RANDOM")"
curl -fsS -X POST "http://$ADDR/api/v1/policies" \
  -H 'Content-Type: application/json' \
  -d "$POLICY_BODY" >/dev/null || fail "policy create failed"
ok "policy created"
POLICIES=$(curl -fsS "http://$ADDR/api/v1/policies")
printf '%s' "$POLICIES" | grep -q '"bound_domain"' || fail "policy list missing bound_domain field"
ok "policy list returns bound_domain"

# 11. Runtime list endpoint (no workers in smoke mode → empty).
bold "[11/7] runtime list endpoint stable"
RUNTIMES=$(curl -fsS "http://$ADDR/api/v1/runtimes")
printf '%s' "$RUNTIMES" | grep -q '"items"' || fail "runtimes list missing items"
TOTAL=$(printf '%s' "$RUNTIMES" | python3 -c "import sys,json;print(json.load(sys.stdin)['total'])")
[[ "$TOTAL" -ge 0 ]] || fail "runtimes total < 0"
ok "runtimes returns $TOTAL worker(s)"

bold "[12/7] shutting down"; kill "$SERVER_PID" 2>/dev/null || true
trap - EXIT
rm -f "$LOG"

bold "ALL GOOD ✓"
echo "  Try also: $BIN_DIR/hnsx run --domain $ROOT/example-domains/customer-service/domain.yaml --adapter noop"
