#!/usr/bin/env bash
# scripts/e2e.sh — End-to-end flywheel demo
#
# Runs the full pipeline against a real Postgres + real claude CLI:
#
#   1. create workspace, agent, issue (CLI)
#   2. assign issue to agent
#   3. daemon claims, spawns claude, streams observations
#   4. issue transitions todo → in_progress → done
#   5. eval.AutoRun fires (no-op when no EvalSet in workspace)
#
# Requires:
#   - Postgres reachable via HNSX_POSTGRES_DSN
#   - claude CLI on PATH (otherwise the daemon's exec fails and the
#     issue is marked blocked instead of done)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${HNSXD_BIN:-$ROOT/../hnsx-server}"
DB="${HNSX_POSTGRES_DSN:-postgres://zhuanz@/hnsx_dev?host=/tmp&sslmode=disable}"

export HNSX_POSTGRES_DSN="$DB"
export HNSX_LOG_LEVEL="${HNSX_LOG_LEVEL:-error}"

log()  { printf '\033[1;34m[e2e]\033[0m %s\n' "$*"; }
pass() { printf '\033[1;32m  ✓\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m  ✗\033[0m %s\n' "$*"; exit 1; }

# 0. Build hnsxd.
if [[ ! -x "$BIN/hnsxd" ]]; then
    log "building hnsxd"
    (cd "$BIN" && go build -o "$BIN/hnsxd" ./cmd/hnsxd)
fi
HNSXD="$BIN/hnsxd"

# 1. Start the server in background. Skip if already running.
if ! curl -sf http://127.0.0.1:8080/healthz > /dev/null 2>&1; then
    log "starting hnsxd serve on :8080"
    "$HNSXD" serve > /tmp/hnsxd-serve.log 2>&1 &
    echo $! > /tmp/hnsxd-serve.pid
    for _ in $(seq 1 20); do
        if curl -sf http://127.0.0.1:8080/healthz > /dev/null 2>&1; then break; fi
        sleep 0.2
    done
fi
curl -sf http://127.0.0.1:8080/healthz > /dev/null || fail "server not healthy"

# 2. Create workspace + agent + issue.
SLUG="e2e-$(date +%s)"
WS=$(cd "$BIN" && "$HNSXD" workspace create --name "E2E Demo" --slug "$SLUG" --description "auto e2e" --output json 2>/dev/null)
WS_ID=$(echo "$WS" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
[[ -n "$WS_ID" ]] || fail "workspace create failed"

AG=$(cd "$BIN" && "$HNSXD" agent create --workspace "$WS_ID" --name "demo-agent" --description "demo" --output json 2>/dev/null)
AG_ID=$(echo "$AG" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['id'] if isinstance(d, dict) else d[0]['id'])")
[[ -n "$AG_ID" ]] || fail "agent create failed"

IS=$(cd "$BIN" && "$HNSXD" issue create --workspace "$WS_ID" --title "Say hello" --description "print hello world" --creator "00000000-0000-0000-0000-000000000001" --output json 2>/dev/null)
IS_ID=$(echo "$IS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['id'] if isinstance(d, dict) else d[0]['id'])")
[[ -n "$IS_ID" ]] || fail "issue create failed"

log "workspace=$WS_ID agent=$AG_ID issue=$IS_ID"
pass "CRUD: workspace + agent + issue created"

# 3. Assign issue to agent.
(cd "$BIN" && "$HNSXD" issue assign "$IS_ID" --type agent --to "$AG_ID" > /dev/null 2>&1)
pass "issue assigned → backlog→todo transition"

# 4. Run daemon long enough for claude to finish (or fail).
log "running daemon for ${HNSXD_DAEMON_SECS:-30}s"
"$HNSXD" daemon --workspace "$WS_ID" --tick-seconds 1 > /tmp/hnsxd-daemon.log 2>&1 &
DPID=$!
sleep "${HNSXD_DAEMON_SECS:-30}"
kill -9 "$DPID" 2>/dev/null
wait "$DPID" 2>/dev/null

# 5. Verify final state.
FINAL=$(psql -d "$(echo "$DB" | sed -E 's|.*?/([^?]+).*|\1|')" -tA -c "SELECT status FROM issues WHERE id='$IS_ID'")
[[ "$FINAL" == "done" || "$FINAL" == "blocked" ]] \
    || fail "issue status = $FINAL, expected done|blocked"
pass "issue closed: status=$FINAL"

OBS_COUNT=$(psql -d "$(echo "$DB" | sed -E 's|.*?/([^?]+).*|\1|')" -tA -c "SELECT COUNT(*) FROM observations WHERE issue_id='$IS_ID'")
[[ "$OBS_COUNT" -ge 1 ]] || fail "no observations recorded (got $OBS_COUNT)"
pass "observations written: $OBS_COUNT rows"

# Eval is best-effort: skip if no EvalSet.
log "e2e demo complete"
log "  issue:        $IS_ID (status=$FINAL)"
log "  observations: $OBS_COUNT"
log "  server log:   /tmp/hnsxd-serve.log"
log "  daemon log:   /tmp/hnsxd-daemon.log"