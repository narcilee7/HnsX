SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c

ROOT        := $(shell pwd)
SERVER_DIR  := $(ROOT)/hnsx-server
CONSOLE_DIR := $(ROOT)/hnsx-console
PROTO_DIR   := $(ROOT)/proto
BIN_DIR     := $(ROOT)/bin
DEPLOY_DIR  := $(ROOT)/deployments/local
WORKER_DIR  := $(ROOT)/hnsx-worker
GOBIN       := $(shell go env GOPATH 2>/dev/null)/bin

# Ensure protoc plugins installed via `go install` are on PATH for buf.
# Buffer looks for `protoc-gen-*` binaries when expanding `local:` plugins.
PROTO_PATH := PATH="$(GOBIN):$$PATH"

# Python worker (V1.1) — venv-relative bin dir; proto gen needs
# protoc-gen-python and protoc-gen-grpc-python on PATH (provided by
# `pip install grpcio-tools` inside the worker venv).
PYTHON      ?= python3
VENV_DIR    := $(WORKER_DIR)/.venv
VENV_BIN    := $(VENV_DIR)/bin
PY_PROTO_PATH := PATH="$(VENV_BIN):$$PATH"

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
	@echo "  proto           - run buf lint + buf generate (Go only)"
	@echo "  proto-py        - regenerate Python proto stubs (worker)"
	@echo "  proto-all       - regenerate Go + Python proto stubs"
	@echo "  test            - go vet + go test (server)"
	@echo "  type-check      - tsc --noEmit (console)"
	@echo "  lint            - go vet + console eslint"
	@echo "  worker-install  - create venv + pip install hnsx-worker editable"
	@echo "  worker-test     - run hnsx-worker pytest"
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
# Python worker (V1.1)
# ---------------------------------------------------------------------------

.PHONY: worker-install
worker-install:
	@if [ ! -d $(VENV_DIR) ]; then $(PYTHON) -m venv $(VENV_DIR); fi
	$(VENV_BIN)/pip install --upgrade pip
	$(VENV_BIN)/pip install grpcio-tools
	cd $(WORKER_DIR) && $(VENV_BIN)/pip install -e ".[dev]"

.PHONY: worker-test
worker-test:
	cd $(WORKER_DIR) && $(VENV_BIN)/pytest -q

.PHONY: worker-import-check
worker-import-check:
	cd $(WORKER_DIR) && $(VENV_BIN)/python -c \
	  "from hnsx_worker.proto.gen.hnsx.v1 import worker_pb2, worker_pb2_grpc; \
	   assert len(worker_pb2.DESCRIPTOR.services_by_name) == 2, 'expected WorkerService + SchedulerService'; \
	   print('ok')"

.PHONY: proto-py
proto-py:
	mkdir -p $(WORKER_DIR)/hnsx_worker/proto/gen
	cd $(PROTO_DIR) && $(PY_PROTO_PATH) python -m grpc_tools.protoc \
		-I. \
		--python_out=../hnsx-worker/hnsx_worker/proto/gen \
		--grpc_python_out=../hnsx-worker/hnsx_worker/proto/gen \
		--pyi_out=../hnsx-worker/hnsx_worker/proto/gen \
		hnsx/v1/*.proto
	@# Rewrite the cross-file imports in generated *_pb2.py and *_pb2_grpc.py
	@# files so they resolve to the actual package layout
	@# (hnsx_worker.proto.gen.hnsx.v1.*). Without this, the generated code
	@# does ``from hnsx.v1 import X_pb2``, but the file lives at
	@# hnsx_worker/proto/gen/hnsx/v1/X_pb2.py.
	@find $(WORKER_DIR)/hnsx_worker/proto/gen \( -name '*_pb2.py' -o -name '*_pb2_grpc.py' \) -exec \
		sed -i '' -E 's|^from hnsx(\.v1)? import (.+)$$|from hnsx_worker.proto.gen.hnsx.v1 import \2|' {} +
	@find $(WORKER_DIR)/hnsx_worker/proto/gen \( -name '*_pb2.py' -o -name '*_pb2_grpc.py' \) -exec \
		sed -i '' -E 's|^import hnsx(\.v1)?$$|import hnsx_worker.proto.gen.hnsx.v1 as hnsx_dot_v1|' {} +

.PHONY: proto-all
proto-all: proto proto-py

.PHONY: worker-clean
worker-clean:
	rm -rf $(VENV_DIR) $(WORKER_DIR)/build $(WORKER_DIR)/dist \
	       $(WORKER_DIR)/hnsx_worker/proto/gen $(WORKER_DIR)/*.egg-info \
	       $(WORKER_DIR)/.pytest_cache $(WORKER_DIR)/.mypy_cache $(WORKER_DIR)/.ruff_cache

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
