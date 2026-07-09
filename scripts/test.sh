#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER_DIR="$ROOT/hnsx-server"

cd "$SERVER_DIR"

echo "==> go vet"
go vet ./...

echo "==> go test"
go test ./...

echo "==> gofmt check"
GOFMT_DIFF="$(gofmt -l .)"
if [[ -n "$GOFMT_DIFF" ]]; then
  echo "files need formatting:" >&2
  echo "$GOFMT_DIFF" >&2
  exit 1
fi
