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

# 3. Validate the example domains.
bold "[3/7] validating example domains"
for dir in "$ROOT/example-domains"/*/; do
  [[ -f "$dir/domain.yaml" ]] || continue
  out="$("$BIN_DIR/hnsx" validate --domain "$dir/domain.yaml" --json)"
  valid=$(printf '%s' "$out" | grep -o '"valid":[ ]*true' || true)
  [[ -n "$valid" ]] || fail "domain $(basename "$dir") failed validation: $out"
  ok "$(basename "$dir")"
done

# 4. Boot server with explicit --seed-from so we have known fixtures.
LOG="$(mktemp -t hnsx-smoke.XXXXXX.log)"
bold "[4/7] booting server on $ADDR (log $LOG)"
HNSX_HTTP_ADDR="$ADDR" HNSX_GRPC_ADDR="" "$BIN_DIR/hnsx-server" server --seed-from "$ROOT/example-domains" >"$LOG" 2>&1 &
SERVER_PID=$!
trap 'kill "$SERVER_PID" 2>/dev/null || true; rm -f "$LOG"' EXIT

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

bold "[7/7] shutting down"; kill "$SERVER_PID" 2>/dev/null || true
trap - EXIT
rm -f "$LOG"

bold "ALL GOOD ✓"
echo "  Try also: $BIN_DIR/hnsx run --domain $ROOT/example-domains/customer-service/domain.yaml --adapter noop"
