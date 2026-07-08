#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

build_go() {
  echo "==> Building Go binaries..."
  cd "$ROOT/go"
  go build -o "$ROOT/bin/hnsx" ./cmd/hnsx
  go build -o "$ROOT/bin/hnsx-server" ./cmd/hnsx-server
}

build_console() {
  echo "==> Building React Console..."
  cd "$ROOT/console"
  pnpm install
  pnpm build
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
