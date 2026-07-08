.PHONY: build build-cli build-server build-console test test-go type-check-console clean dev-console proto

VERSION ?= 0.2.0

build: build-cli build-server

build-cli:
	cd go && go build -o ../bin/hnsx ./cmd/hnsx

build-server:
	cd go && go build -o ../bin/hnsx-server ./cmd/hnsx-server

build-console:
	cd console && pnpm install && pnpm build

test: test-go

test-go:
	cd go && go test ./...

type-check-console:
	cd console && pnpm type-check

lint-go:
	cd go && go vet ./...

dev-console:
	cd console && pnpm dev

clean:
	rm -rf bin go/hnsx go/hnsx-server

proto:
	mkdir -p proto/gen/go proto/gen/ts proto/gen/python
	# TODO: wire buf or protoc generation once generators are installed.
	@echo "Proto generation not yet wired. Add buf or protoc commands here."
