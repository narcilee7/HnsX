#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "==> go vet"
cd "$ROOT/hnsx-server" && go vet ./...

echo "==> go test"
cd "$ROOT/hnsx-server" && go test ./...

echo "==> gofmt check"
GOFMT_DIFF="$(gofmt -l "$ROOT/hnsx-server" "$ROOT/sdk/go")"
if [[ -n "$GOFMT_DIFF" ]]; then
  echo "files need formatting:" >&2
  echo "$GOFMT_DIFF" >&2
  exit 1
fi
