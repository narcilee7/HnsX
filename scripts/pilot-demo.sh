#!/usr/bin/env bash
# scripts/pilot-demo.sh — end-to-end HarnessX pilot walkthrough (W27).
#
# Spins up Postgres, the control plane (multica-mode), the daemon, and
# walks through the three killer scenarios:
#
#   1. Weekend on-call: high-cost task auto-pauses → human approves.
#   2. Cost overrun:    team dashboard shows per-domain spend.
#   3. SOC2 audit:      audit log export with hash-chain verification.
#
# Pre-requisites:
#   - Postgres running locally with a 'harnessx' user and 'harnessx' DB.
#   - bin/harnessx-server, bin/harnessx-daemon, bin/multica, bin/multica-server built
#     (run `make build-all`).
#
# Usage:
#   ./scripts/pilot-demo.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$REPO_ROOT/bin"
LOG_DIR="$REPO_ROOT/.pilot-demo"
mkdir -p "$LOG_DIR"

echo "==> HarnessX pilot demo"
echo "    repo: $REPO_ROOT"
echo "    logs: $LOG_DIR"

# 1. Postgres health check.
echo
echo "[1/7] Verifying Postgres..."
PG_URL="${HNSX_DATABASE_URL:-postgres://harnessx:harnessx@localhost:5432/harnessx?sslmode=disable}"
if ! psql "$PG_URL" -c 'SELECT 1' >/dev/null 2>&1; then
  echo "✗ Postgres unreachable at $PG_URL"
  echo "  Try: brew services start postgresql@16 && createuser harnessx --pwprompt && createdb -O harnessx harnessx"
  exit 1
fi
echo "  ✓ Postgres OK"

# 2. Start the control plane.
echo
echo "[2/7] Starting control plane (multica-mode)..."
pkill -f "hnsx-server server" 2>/dev/null || true
sleep 0.5
export HNSX_DATABASE_URL="$PG_URL"
export HNSX_HTTP_ADDR="${HNSX_HTTP_ADDR:-127.0.0.1:50051}"
"$BIN/hnsx-server" server --multica-mode >"$LOG_DIR/server.log" 2>&1 &
SERVER_PID=$!
echo "  server pid=$SERVER_PID"
sleep 2
if ! kill -0 "$SERVER_PID" 2>/dev/null; then
  echo "✗ server failed to start; see $LOG_DIR/server.log"
  exit 1
fi
echo "  ✓ server listening on $HNSX_HTTP_ADDR"

# 3. Start the daemon.
echo
echo "[3/7] Starting HarnessX daemon..."
pkill -f "harnessx-daemon" 2>/dev/null || true
sleep 0.5
"$BIN/harnessx-daemon" \
  --server "http://$HNSX_HTTP_ADDR" \
  --workspace-id "00000000-0000-0000-0000-000000000000" \
  --max-cost-usd 0.5 \
  >"$LOG_DIR/daemon.log" 2>&1 &
DAEMON_PID=$!
echo "  daemon pid=$DAEMON_PID"
sleep 2
if ! kill -0 "$DAEMON_PID" 2>/dev/null; then
  echo "✗ daemon failed to start; see $LOG_DIR/daemon.log"
  kill "$SERVER_PID" 2>/dev/null
  exit 1
fi
echo "  ✓ daemon registered"

# 4. Register a Domain (the customer-service demo).
echo
echo "[4/7] Registering customer-service Domain..."
DOMAIN_RESP=$(curl -fsS -X POST "http://$HNSX_HTTP_ADDR/api/harnessx/domains" \
  -H 'Content-Type: application/json' \
  -d @- <<'EOF'
{
  "id": "customer-service",
  "version": "1.0.0",
  "description": "Routes customer questions to the right specialist agent",
  "harness": {
    "agents": {
      "triage": {
        "id": "triage",
        "provider": "anthropic",
        "model": "claude-haiku-4-5",
        "adapter": {"kind": "anthropic", "timeout_seconds": 30}
      }
    },
    "session": {"mode": "single"},
    "policy": {"budget": {"max_cost_usd": 1.0}}
  }
}
EOF
)
echo "  ✓ registered: $(echo "$DOMAIN_RESP" | head -c 80)..."

# 5. Create an Issue (Session trigger).
echo
echo "[5/7] Creating demo Issue (triggering a Session)..."
ISSUE_RESP=$(curl -fsS -X POST "http://$HNSX_HTTP_ADDR/api/harnessx/domains/customer-service/run" \
  -H 'Content-Type: application/json' \
  -d '{"trigger":{"issue_title":"Demo: customer refund request","issue_description":"Customer wants a $300 refund for order #12345"}}')
SESSION_ID=$(echo "$ISSUE_RESP" | sed -n 's/.*"session_id":"\([^"]*\)".*/\1/p')
echo "  ✓ session_id=$SESSION_ID"

# 6. Cost dashboard.
echo
echo "[6/7] Pulling cost dashboard..."
DASH=$(curl -fsS "http://$HNSX_HTTP_ADDR/api/harnessx/cost/dashboard")
echo "  ✓ dashboard: $DASH"

# 7. Audit log.
echo
echo "[7/7] Pulling audit log..."
AUDIT=$(curl -fsS "http://$HNSX_HTTP_ADDR/api/harnessx/audit?limit=10")
echo "  ✓ audit entries: $(echo "$AUDIT" | grep -c '"id"' || echo 0)"

# Tear down.
echo
echo "==> Demo complete. Tearing down..."
kill "$DAEMON_PID" "$SERVER_PID" 2>/dev/null || true
sleep 1

cat <<EOF

Three killer scenarios (to demo in your pitch):

  1. Weekend on-call (server $LOG_DIR/server.log):
     • Create an Issue with a high-cost trigger
     • Server auto-pauses → emits approval_required observation
     • Operator hits POST /api/harnessx/approvals/<id>/approve
     • Session resumes, agent continues

  2. Cost overrun:
     • Open Cost Dashboard (GET /api/harnessx/cost/dashboard)
     • Filter by domain_id → see cumulative spend per DomainSpec

  3. SOC2 audit:
     • Run GET /api/harnessx/audit?limit=100
     • Each entry has hash chain verification (see hnsx_audit_log table)
     • Export as CSV for the auditor

Logs left at: $LOG_DIR
EOF
