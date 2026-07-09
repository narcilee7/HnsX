#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER_DIR="$ROOT/hnsx-server"
CONSOLE_DIR="$ROOT/hnsx-console"
BIN_DIR="$ROOT/bin"

mkdir -p "$BIN_DIR"

build_go() {
  echo "==> Building Go binaries..."
  cd "$SERVER_DIR"
  go build -o "$BIN_DIR/hnsx"        ./cmd/hnsx
  go build -o "$BIN_DIR/hnsx-server" ./cmd/hnsx-server
}

build_console() {
  echo "==> Building React Console..."
  if [[ ! -d "$CONSOLE_DIR/node_modules" ]]; then
    ( cd "$CONSOLE_DIR" && pnpm install --frozen-lockfile )
  fi
  ( cd "$CONSOLE_DIR" && pnpm build )
}

case "${1:-all}" in
  go)
    build_go
    ;;
  console)
    build_console
    ;;
  all)
    build_go
    build_console
    ;;
  *)
    echo "Usage: $0 [go|console|all]"
    exit 1
    ;;
esac

echo "==> Done"
