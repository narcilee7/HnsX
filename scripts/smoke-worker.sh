#!/usr/bin/env bash
# scripts/smoke-worker.sh — end-to-end smoke for the V1.1 Python Worker Pivot.
#
# Boots hnsx-server (HTTP + gRPC), starts a Python worker, triggers the
# noop-smoke domain, and verifies the full CLI → server → worker → SSE
# observation pipeline.
#
# Usage:
#   ./scripts/smoke-worker.sh
#
# Requires:
#   - bin/hnsx-server (run `make build` first)
#   - hnsx-worker/.venv with hnsx-worker installed (run `make worker-install`)
#   - curl
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="$ROOT/bin"
HTTP_ADDR="127.0.0.1:51080"
GRPC_ADDR="127.0.0.1:51081"

bold() { printf "\033[1m%s\033[0m\n" "$*"; }
ok() { printf "  \033[32m✓\033[0m %s\n" "$*"; }
fail() { printf "  \033[31m✗\033[0m %s\n" "$*"; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || fail "missing dependency: $1"
}

need curl
need python3

SERVER_BIN="$BIN_DIR/hnsx-server"
[[ -x "$SERVER_BIN" ]] || fail "missing $SERVER_BIN; run 'make build' first"

VENV_PYTHON="$ROOT/hnsx-worker/.venv/bin/python"
[[ -x "$VENV_PYTHON" ]] || fail "missing $VENV_PYTHON; run 'make worker-install' first"

SERVER_LOG="$(mktemp -t hnsx-smoke-server.XXXXXX.log)"
WORKER_LOG="$(mktemp -t hnsx-smoke-worker.XXXXXX.log)"
SSE_FILE="$(mktemp -t hnsx-smoke-sse.XXXXXX.log)"

cleanup() {
  [[ -n "${SERVER_PID:-}" ]] && kill "$SERVER_PID" 2>/dev/null || true
  [[ -n "${WORKER_PID:-}" ]] && kill "$WORKER_PID" 2>/dev/null || true
  rm -f "$SERVER_LOG" "$WORKER_LOG" "$SSE_FILE"
}
trap cleanup EXIT

bold "[1/5] booting hnsx-server (http=$HTTP_ADDR grpc=$GRPC_ADDR)"
HNSX_HTTP_ADDR="$HTTP_ADDR" HNSX_GRPC_ADDR="$GRPC_ADDR" \
  "$SERVER_BIN" server --seed-from "$ROOT/example-domains" >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

for i in $(seq 1 50); do
  if curl -fsS "http://$HTTP_ADDR/healthz" >/dev/null 2>&1; then break; fi
  sleep 0.1
done
curl -fsS "http://$HTTP_ADDR/healthz" >/dev/null || { cat "$SERVER_LOG"; fail "server failed to start"; }
ok "server healthy"

bold "[2/5] starting Python worker"
(
  cd "$ROOT/hnsx-worker"
  source .venv/bin/activate
  hnsx-worker run --server "$GRPC_ADDR" --worker-id w-smoke --max-concurrent-sessions 2 --providers noop
) >"$WORKER_LOG" 2>&1 &
WORKER_PID=$!

for i in $(seq 1 50); do
  if grep -q "worker w-smoke ready" "$WORKER_LOG" 2>/dev/null; then break; fi
  sleep 0.1
done
grep -q "worker w-smoke ready" "$WORKER_LOG" 2>/dev/null || { cat "$WORKER_LOG"; fail "worker failed to start"; }
ok "worker ready"

bold "[3/5] triggering noop-smoke session"
RESP=$(curl -fsS -X POST "http://$HTTP_ADDR/api/v1/sessions" \
  -H 'Content-Type: application/json' \
  -d '{"domain_id":"noop-smoke","trigger":{"message":"hello worker"}}')
SID=$(printf '%s' "$RESP" | grep -o '"id":"[^"]*' | head -1 | sed 's/^"id":"//')
[[ -n "$SID" ]] || fail "no session id in $RESP"
ok "session created: $SID"

bold "[4/5] capturing SSE observations"
curl -N -fsS "http://$HTTP_ADDR/api/v1/sessions/$SID/events" >"$SSE_FILE" 2>/dev/null &
SSE_PID=$!
sleep 2.0
kill "$SSE_PID" 2>/dev/null || true
wait "$SSE_PID" 2>/dev/null || true

EVENTS=$(grep -c '^event:' "$SSE_FILE" || true)
[[ "$EVENTS" -ge 4 ]] || fail "expected ≥4 SSE events, got $EVENTS"
ok "captured $EVENTS SSE events"

grep -q 'event: observation' "$SSE_FILE" || fail "no observation events"
grep -q 'agent_text' "$SSE_FILE" || fail "no agent_text observation"
grep -q 'session_end' "$SSE_FILE" || fail "no session_end observation"
ok "observation kinds: agent_text, session_end present"

bold "[5/5] verifying session completed"
FINAL=$(curl -fsS "http://$HTTP_ADDR/api/v1/sessions/$SID")
STATE=$(printf '%s' "$FINAL" | grep -o '"state":"[^"]*' | head -1 | sed 's/^"state":"//')
[[ "$STATE" == "completed" ]] || fail "session did not complete (state=$STATE)"
ok "session state=completed"

bold "ALL GOOD ✓"
ok "V1.1 worker pipeline: server → worker → SSE → completed"
