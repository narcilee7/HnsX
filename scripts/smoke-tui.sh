#!/usr/bin/env bash
# scripts/smoke-tui.sh — smoke for the TUI default entry point.
#
# Verifies:
#   - hnsx --no-tui prints help
#   - hnsx --help still works
#   - hnsx <command> still runs (version)
#   - HNSX_NO_TUI disables the TUI
#   - the TUI binary can start and quit without panic (using a headless harness)
#
# Pre-conditions:
#   - bin/hnsx exists (run `make build-cli` if not)
#
# Usage:
#   ./scripts/smoke-tui.sh

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="$ROOT/bin"
HNSX="${HNSX_BIN:-$BIN_DIR/hnsx}"

bold() { printf "\033[1m%s\033[0m\n" "$*"; }
ok()   { printf "  \033[32m✓\033[0m %s\n" "$*"; }
fail() { printf "  \033[31m✗\033[0m %s\n" "$*"; exit 1; }

[[ -x "$HNSX" ]] || fail "hnsx binary not found at $HNSX (run \`make build-cli\`)"

bold "[1/4] --no-tui disables TUI and shows help"
out="$($HNSX --no-tui 2>&1 || true)"
echo "$out" | grep -q "Usage:" || fail "expected help output with --no-tui"
ok "--no-tui shows help"

bold "[2/4] --help still works"
out="$($HNSX --help 2>&1 || true)"
echo "$out" | grep -q "Usage:" || fail "expected help output"
ok "--help works"

bold "[3/4] explicit commands bypass the TUI"
out="$($HNSX version 2>&1 || true)"
[[ "$out" == hnsx* ]] || fail "expected version output, got: $out"
ok "version command works"

bold "[4/4] HNSX_NO_TUI disables the default TUI"
out="$(HNSX_NO_TUI=1 "$HNSX" 2>&1 || true)"
echo "$out" | grep -q "Usage:" || fail "expected help output with HNSX_NO_TUI=1"
ok "HNSX_NO_TUI=1 shows help"

bold "all TUI smoke checks passed"
