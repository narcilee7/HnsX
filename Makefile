SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c

ROOT        := $(shell pwd)
SERVER_DIR  := $(ROOT)/hnsx-server
CONSOLE_DIR := $(ROOT)/hnsx-console
PROTO_DIR   := $(ROOT)/proto
BIN_DIR     := $(ROOT)/bin
DEPLOY_DIR  := $(ROOT)/deployments/local
GOBIN       := $(shell go env GOPATH 2>/dev/null)/bin

# Ensure protoc plugins installed via `go install` are on PATH for buf.
# Buffer looks for `protoc-gen-*` binaries when expanding `local:` plugins.
PROTO_PATH := PATH="$(GOBIN):$$PATH"

VERSION ?= 0.2.0

# ---------------------------------------------------------------------------
# Version stamping
# ---------------------------------------------------------------------------
VERSION_PKG := github.com/hnsx-io/hnsx/core/version

LDFLAGS := -s -w \
  -X '$(VERSION_PKG).Version=$(VERSION)' \
  -X '$(VERSION_PKG).Commit=$(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)' \
  -X '$(VERSION_PKG).Built=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'

# ---------------------------------------------------------------------------
# Top-level aliases
# ---------------------------------------------------------------------------
.PHONY: help
help:
	@echo "HnsX top-level make targets:"
	@echo "  build           - build CLI + server + console"
	@echo "  proto           - run buf lint + buf generate"
	@echo "  test            - go vet + go test (server)"
	@echo "  type-check      - tsc --noEmit (console)"
	@echo "  lint            - go vet + console eslint"
	@echo "  db-up/db-down   - local Postgres (deployments/local)"
	@echo "  smoke           - end-to-end smoke (requires db-up)"
	@echo "  clean           - remove build artifacts"

.PHONY: build
build: build-cli build-server build-console

# ---------------------------------------------------------------------------
# Go build
# ---------------------------------------------------------------------------
.PHONY: build-cli
build-cli:
	cd hnsx && go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/hnsx ./cmd/hnsx

.PHONY: build-server
build-server:
	cd $(SERVER_DIR) && go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/hnsx-server ./cmd/hnsx-server

.PHONY: build-go
build-go: build-cli build-server

# ---------------------------------------------------------------------------
# Console
# ---------------------------------------------------------------------------
.PHONY: build-console
build-console:
	cd $(CONSOLE_DIR) && pnpm install --frozen-lockfile && pnpm build

.PHONY: dev-console
dev-console:
	cd $(CONSOLE_DIR) && pnpm dev

.PHONY: type-check-console
type-check-console:
	cd $(CONSOLE_DIR) && pnpm type-check

# ---------------------------------------------------------------------------
# Tests / lint
# ---------------------------------------------------------------------------
.PHONY: test
test: vet test-go

.PHONY: test-go
test-go:
	cd hnsx-core && go test ./...
	cd hnsx && go test ./...
	cd $(SERVER_DIR) && go test ./...

.PHONY: vet
vet:
	cd hnsx-core && go vet ./...
	cd $(SERVER_DIR) && go vet ./...

.PHONY: fmt
fmt:
	cd $(SERVER_DIR) && gofmt -w .

.PHONY: lint
lint: vet
	cd $(CONSOLE_DIR) && pnpm lint || true

# ---------------------------------------------------------------------------
# Proto
# ---------------------------------------------------------------------------
.PHONY: proto
proto: proto-lint proto-gen

.PHONY: proto-lint
proto-lint:
	cd $(PROTO_DIR) && buf lint

.PHONY: proto-gen
proto-gen:
	cd $(PROTO_DIR) && $(PROTO_PATH) buf generate

.PHONY: proto-breaking
proto-breaking:
	cd $(PROTO_DIR) && buf breaking --against '.git#branch=main'

# ---------------------------------------------------------------------------
# Local infra (Postgres + Tempo + Grafana)
# ---------------------------------------------------------------------------
.PHONY: db-up
db-up:
	cd $(DEPLOY_DIR) && docker compose up -d postgres

.PHONY: db-down
db-down:
	cd $(DEPLOY_DIR) && docker compose down

.PHONY: tempo-up
tempo-up:
	cd $(DEPLOY_DIR) && docker compose up -d tempo grafana

.PHONY: down
down:
	cd $(DEPLOY_DIR) && docker compose down

# ---------------------------------------------------------------------------
# E2E smoke
# ---------------------------------------------------------------------------
.PHONY: smoke
smoke: build-cli build-server
	./scripts/smoke.sh

# ---------------------------------------------------------------------------
# Clean
# ---------------------------------------------------------------------------
.PHONY: clean
clean:
	rm -rf $(BIN_DIR) $(PROTO_DIR)/gen $(SERVER_DIR)/coverage.out
