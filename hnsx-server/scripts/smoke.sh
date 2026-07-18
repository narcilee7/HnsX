#!/usr/bin/env bash
# scripts/smoke.sh — R1.x smoke for hnsxd
#
# Runs the smallest set of checks that prove the rewrite skeleton is healthy:
#   1. `go build ./...` succeeds (catches type / import errors)
#   2. `go test ./...` passes (catches domain port / parser regressions)
#   3. `hnsxd --help` boots the cobra root
#   4. `hnsxd backends list` reports `claude` (proves app.New wires infra)
#   5. grep blacklists are all 0 (proves no leftover old code)
#
# Designed to run on a CI box without Postgres / claude CLI / network.
# Anything that needs DB or a real agent CLI is gated to the integration
# lane (R1.12).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

log()  { printf '\033[1;34m[smoke]\033[0m %s\n' "$*"; }
pass() { printf '\033[1;32m  ✓\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m  ✗\033[0m %s\n' "$*"; exit 1; }

log "1/5  go build ./..."
go build ./... || fail "go build failed"

log "2/5  go test ./..."
go test ./... || fail "go test failed"
pass "go build + test green"

log "3/5  hnsxd boots"
go build -o /tmp/hnsxd ./cmd/hnsxd
output=$(/tmp/hnsxd --help 2>&1) || fail "hnsxd --help exited non-zero"
echo "$output" | grep -q "hnsxd is the single HnsX binary" \
    || fail "hnsxd --help did not print expected banner"
pass "cobra root boots, banner printed"

log "4/5  hnsxd backends list"
output=$(/tmp/hnsxd backends list 2>/dev/null) || fail "hnsxd backends list exited non-zero"
echo "$output" | grep -qx "claude" \
    || fail "expected 'claude' in backend list, got: $output"
count=$(echo "$output" | wc -l | tr -d ' ')
if [[ $count -lt 10 ]]; then
    fail "expected at least 10 backends registered, got $count: $output"
fi
pass "backends registered ($count total; claude present)"

log "5/5  grep blacklist (no leftover old code)"
# Run a series of greps that must all return empty. Each line documents
# what we're policing so future readers know why.
blacklist=(
    "multica_adapter"           "no multica adapter code allowed"
    "hnsx_worker"               "no python worker code allowed"
    "chi\."                     "no chi router (we use gin)"
    "connectrpc"                "no connectrpc (no gRPC stack)"
    "google.golang.org/grpc"    "no gRPC"
    "google.golang.org/protobuf" "no protobuf"
    "mat_"                      "no Multica token prefix"
)
all_ok=1
for i in $(seq 0 2 $((${#blacklist[@]} - 1))); do
    pattern="${blacklist[$i]}"
    reason="${blacklist[$((i + 1))]}"
    if grep -rq --include="*.go" "$pattern" . 2>/dev/null; then
        printf '\033[1;31m  ✗\033[0m blacklist hit: %s (%s)\n' "$pattern" "$reason"
        grep -rn --include="*.go" "$pattern" . | head -5 | sed 's/^/      /'
        all_ok=0
    fi
done
[[ $all_ok -eq 1 ]] || fail "blacklist violations above"
pass "no leftover old code"

log "all R1.x smoke checks passed"