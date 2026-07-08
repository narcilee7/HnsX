# HnsX — Makefile
# Common tasks for the Rust workspace and optional web frontend.

.PHONY: all build release test check fmt lint run-web run-cli docker help

all: build test

build:
	cargo build --workspace

release:
	cargo build --workspace --release

test:
	cargo test --workspace

# Fast feedback loop: format + clippy + test
check: fmt lint test

fmt:
	cargo fmt --all

lint:
	cargo clippy --workspace -- -D warnings

# Build and run the web UI (assumes pnpm is installed)
run-web:
	cd web && pnpm install && pnpm dev

# Run the CLI in a convenient debug mode
run-cli:
	cargo run --package hnsx-cli -- --help

# Package a domain into a deployable artifact
test-package:
	cargo run --package hnsx-cli -- package domains/code-review --output /tmp/code-review.hnsx

# Docker image for the control-plane + web bundle
docker:
	docker build -t hnsx:latest -f docker/Dockerfile .

# Clean build artifacts
clean:
	cargo clean
	rm -rf web/dist

help:
	@echo "Available targets:"
	@echo "  build       - cargo build (debug)"
	@echo "  release     - cargo build (release)"
	@echo "  test        - cargo test --workspace"
	@echo "  check       - fmt + lint + test"
	@echo "  fmt         - cargo fmt --all"
	@echo "  lint        - cargo clippy with warnings as errors"
	@echo "  run-web     - install and start the Vite dev server"
	@echo "  run-cli     - show hnsx-cli help"
	@echo "  test-package- package the code-review domain as a sanity check"
	@echo "  docker      - build the hnsx Docker image"
	@echo "  clean       - remove target/ and web/dist"
