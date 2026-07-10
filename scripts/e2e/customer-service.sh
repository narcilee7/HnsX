#!/usr/bin/env bash
# scripts/e2e/customer-service.sh — end-to-end smoke for the customer-service domain.
#
# Flow:
#   1. docker compose up (postgres + tempo + grafana + hnsx-server + worker)
#   2. validate example-domains/customer-service/domain.yaml
#   3. POST deployments/local/customer-service-e2e.yaml (echo-adapter variant)
#   4. trigger a session
#   5. consume SSE observations
#   6. verify trace persisted
#
# Usage:
#   ./scripts/e2e/customer-service.sh
#
# Environment:
#   HNSX_SECRET_KEY  — AES-256-GCM secret key (default: generated)
#   HNSX_OTEL_EXPORTER — stdout | otlp | none (default: otlp)
#   COMPOSE_PROJECT_NAME — docker compose project name (default: hnsx-e2e)

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DEPLOY_DIR="$ROOT/deployments/local"
ADDR="127.0.0.1:50052"

export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-hnsx-e2e}"
export HNSX_SECRET_KEY="${HNSX_SECRET_KEY:-hnsx-e2e-secret-key-do-not-use-in-prod-2026}"
export HNSX_OTEL_EXPORTER="${HNSX_OTEL_EXPORTER:-otlp}"

bold() { printf "\033[1m%s\033[0m\n" "$*"; }
ok() { printf "  \033[32m✓\033[0m %s\n" "$*"; }
fail() { printf "  \033[31m✗\033[0m %s\n" "$*"; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || fail "missing dependency: $1"
}

need docker
need curl
need python3

cleanup() {
  bold "cleaning up docker compose"
  (cd "$DEPLOY_DIR" && docker compose down --volumes) >/dev/null 2>&1 || true
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# 1. Boot the stack.
# ---------------------------------------------------------------------------
bold "[1/6] building + starting docker compose stack"
(cd "$DEPLOY_DIR" && docker compose up -d --build) || fail "docker compose up failed"

bold "[2/6] waiting for hnsx-server /healthz"
for i in $(seq 1 60); do
  if curl -fsS "http://$ADDR/healthz" >/dev/null 2>&1; then break; fi
  sleep 1
done
curl -fsS "http://$ADDR/healthz" >/dev/null || fail "server failed to become healthy"
ok "server healthy"

# ---------------------------------------------------------------------------
# 2. Validate the real customer-service domain (structural check).
# ---------------------------------------------------------------------------
bold "[3/6] validating example-domains/customer-service/domain.yaml"
VALIDATE_RESP=$(curl -fsS -X POST "http://$ADDR/api/v1/domains/customer-service/validate" \
  -H 'Content-Type: application/x-yaml' \
  --data-binary @"$ROOT/example-domains/customer-service/domain.yaml")
printf '%s' "$VALIDATE_RESP" | grep -q '"valid":true' || fail "customer-service validation failed: $VALIDATE_RESP"
ok "customer-service domain valid"

# ---------------------------------------------------------------------------
# 3. Register the echo-adapter e2e variant.
# ---------------------------------------------------------------------------
bold "[4/6] registering customer-service-e2e domain"
DOMAIN_ID="customer-service-e2e"
if curl -fsS "http://$ADDR/api/v1/domains/$DOMAIN_ID" >/dev/null 2>&1; then
  ok "domain already registered: $DOMAIN_ID"
else
  CREATE_RESP=$(curl -fsS -X POST "http://$ADDR/api/v1/domains" \
    -H 'Content-Type: application/x-yaml' \
    --data-binary @"$DEPLOY_DIR/customer-service-e2e.yaml")
  DOMAIN_ID=$(printf '%s' "$CREATE_RESP" | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")
  [[ -n "$DOMAIN_ID" ]] || fail "domain registration returned empty id: $CREATE_RESP"
  ok "domain registered: $DOMAIN_ID"
fi

# ---------------------------------------------------------------------------
# 4. Trigger a session.
# ---------------------------------------------------------------------------
bold "[5/6] triggering session"
TRIGGER_RESP=$(curl -fsS -X POST "http://$ADDR/api/v1/domains/$DOMAIN_ID/run" \
  -H 'Content-Type: application/json' \
  -d '{"trigger":{"message":"My refund has not arrived"}}')
SID=$(printf '%s' "$TRIGGER_RESP" | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")
[[ -n "$SID" ]] || fail "trigger returned empty session id: $TRIGGER_RESP"
ok "session triggered: $SID"

# ---------------------------------------------------------------------------
# 5. Consume SSE observations.
# ---------------------------------------------------------------------------
SSE_FILE="$(mktemp -t hnsx-e2e-sse.XXXXXX.log)"
curl -N -fsS "http://$ADDR/api/v1/sessions/$SID/events" >"$SSE_FILE" 2>/dev/null &
SSE_PID=$!

# Wait for session to complete (poll session state).
for i in $(seq 1 60); do
  STATE=$(curl -fsS "http://$ADDR/api/v1/sessions/$SID" | python3 -c "import sys,json;print(json.load(sys.stdin)['state'])")
  if [[ "$STATE" == "completed" || "$STATE" == "failed" ]]; then break; fi
  sleep 1
done

kill "$SSE_PID" 2>/dev/null || true
wait "$SSE_PID" 2>/dev/null || true

[[ "$STATE" == "completed" ]] || fail "session did not complete (state=$STATE)"
ok "session completed"

EVENTS=$(grep -c '^event:' "$SSE_FILE" || true)
[[ "$EVENTS" -ge 3 ]] || fail "expected ≥3 SSE events, got $EVENTS"
ok "captured $EVENTS SSE events"

OBS_SEQUENCE=$(grep '^data:' "$SSE_FILE" | python3 -c "
import sys, json
kinds = []
for line in sys.stdin:
    line = line.strip()
    if not line.startswith('data:'):
        continue
    payload = line[5:].strip()
    if not payload:
        continue
    try:
        obj = json.loads(payload)
    except json.JSONDecodeError:
        continue
    if isinstance(obj, dict) and 'kind' in obj:
        kinds.append(obj['kind'])
print(' -> '.join(kinds))
")
ok "observation sequence: $OBS_SEQUENCE"

# ---------------------------------------------------------------------------
# 6. Verify trace persisted and console-equivalent API returns it.
# ---------------------------------------------------------------------------
bold "[6/6] verifying trace persistence"
TRACES=$(curl -fsS "http://$ADDR/api/v1/traces?domain=$DOMAIN_ID")
TRACE_COUNT=$(printf '%s' "$TRACES" | python3 -c "import sys,json;print(json.load(sys.stdin)['total'])")
[[ "$TRACE_COUNT" -ge 1 ]] || fail "expected ≥1 trace for $DOMAIN_ID, got $TRACE_COUNT"
ok "trace list returned $TRACE_COUNT trace(s)"

TID=$(printf '%s' "$TRACES" | python3 -c "import sys,json;print(json.load(sys.stdin)['items'][0]['trace_id'])")
TRACE_DETAIL=$(curl -fsS "http://$ADDR/api/v1/traces/$TID")
OBS_COUNT=$(printf '%s' "$TRACE_DETAIL" | python3 -c "import sys,json;print(len(json.load(sys.stdin)['observations']))")
[[ "$OBS_COUNT" -ge 1 ]] || fail "trace detail has no observations"
ok "trace detail returns $OBS_COUNT observation(s)"

bold "E2E PASSED ✓"
printf "trace_id:  %s\n" "$TID"
printf "session:   %s\n" "$SID"
printf "domain:    %s\n" "$DOMAIN_ID"
printf "obs seq:   %s\n" "$OBS_SEQUENCE"

rm -f "$SSE_FILE"
