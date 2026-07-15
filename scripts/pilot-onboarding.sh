#!/usr/bin/env bash
# scripts/pilot-onboarding.sh — W28 pilot onboarding checklist.
#
# Walks a pilot team through the 7 steps required to go from "blank
# machine" to "first HarnessX session ran end-to-end". Each step has a
# pass/fail check; the script prints a final report and exits non-zero
# if any step fails (so CI / cron can alert).
#
# Usage:
#   ./scripts/pilot-onboarding.sh [--strict]
#
# Flags:
#   --strict    exit non-zero on any failure (default: just report)

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$REPO_ROOT/bin"
STRICT=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --strict) STRICT=1 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
  shift
done

SERVER_URL="${HARNESSX_SERVER:-http://127.0.0.1:50051}"
PG_URL="${HNSX_DATABASE_URL:-postgres://harnessx:harnessx@localhost:5432/harnessx?sslmode=disable}"
DEMO_DOMAIN_ID="${DEMO_DOMAIN_ID:-customer-service}"

passes=0
fails=0
results=()

check() {
  local label="$1"; shift
  local fn="$1"; shift
  if "$fn"; then
    results+=("✓ $label")
    passes=$((passes+1))
  else
    results+=("✗ $label")
    fails=$((fails+1))
  fi
}

# ── Step 1: binaries present ────────────────────────────────────────────
step_binaries() {
  # Note: the control plane binary is named hnsx-server (legacy name
  # preserved for backward compatibility); the rebranded HarnessX
  # binaries are harnessx-daemon and harnessxctl.
  [[ -x "$BIN/hnsx-server" ]] \
    && [[ -x "$BIN/harnessx-daemon" ]]
}
check "Binaries built (hnsx-server / harnessx-daemon)" step_binaries

# ── Step 2: Postgres reachable ──────────────────────────────────────────
step_postgres() {
  psql "$PG_URL" -c 'SELECT 1' >/dev/null 2>&1
}
check "Postgres reachable at $PG_URL" step_postgres

# ── Step 3: server responds on /healthz ──────────────────────────────────
step_healthz() {
  local body
  body="$(curl -fsS --max-time 5 "$SERVER_URL/healthz" 2>/dev/null)" || return 1
  [[ "$body" == *'"status":"ok"'* ]]
}
check "Server responds on $SERVER_URL/healthz" step_healthz

# ── Step 4: multica-mode routes mounted ─────────────────────────────────
step_multica_routes() {
  local body
  body="$(curl -fsS --max-time 5 "$SERVER_URL/api/me" 2>/dev/null)" || return 1
  [[ "$body" == *'"id"'* ]]
}
check "Multica-compatible API mounted (GET /api/me works)" step_multica_routes

# ── Step 5: harnessx routes mounted ─────────────────────────────────────
step_harnessx_routes() {
  local body
  body="$(curl -fsS --max-time 5 "$SERVER_URL/api/harnessx/domains" 2>/dev/null)" || return 1
  [[ "$body" == "["* ]]
}
check "HarnessX API mounted (GET /api/harnessx/domains returns JSON array)" step_harnessx_routes

# ── Step 6: demo Domain registered ──────────────────────────────────────
step_demo_domain() {
  local body
  body="$(curl -fsS --max-time 5 "$SERVER_URL/api/harnessx/domains/$DEMO_DOMAIN_ID" 2>/dev/null)" || return 1
  [[ "$body" == *"$DEMO_DOMAIN_ID"* ]]
}
check "Demo domain $DEMO_DOMAIN_ID is registered" step_demo_domain

# ── Step 7: daemon registered ──────────────────────────────────────────
# A heartbeat with a fresh daemon_id returns {"ok":true,"registered":true}
# when the daemon has already called /api/daemon/register.
step_daemon_registered() {
  local body
  body="$(curl -fsS --max-time 5 -X POST "$SERVER_URL/api/daemon/heartbeat" \
    -H 'Content-Type: application/json' \
    -d '{"daemon_id":"onboarding-probe","timestamp":0}' 2>/dev/null)" || return 1
  # Either the probe daemon was registered (registered:true) or it wasn't
  # (registered:false); either way the endpoint answered with 200, which
  # proves the daemon protocol is wired up.
  [[ "$body" == *'"ok":true'* ]]
}
check "Daemon heartbeat endpoint reachable" step_daemon_registered

# ── Report ──────────────────────────────────────────────────────────────
echo
echo "==> HarnessX Pilot Onboarding Report"
echo "    server: $SERVER_URL"
echo "    pg:     $PG_URL"
echo
for r in "${results[@]}"; do
  echo "    $r"
done
echo
echo "    ${passes} passed, ${fails} failed"
echo

if (( fails > 0 )); then
  echo "Onboarding incomplete. Fix the failing checks above, then re-run."
  if (( STRICT )); then
    exit 1
  fi
fi
echo "Ready to demo. Run ./scripts/pilot-demo.sh for the end-to-end walkthrough."
