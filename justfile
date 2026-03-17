# Alloy Justfile - Task Runner

# Set shell
set shell := ["bash", "-c"]

# Default task
default: help

# Show help
help:
    @just --list

# Format code
fmt:
    go fmt ./...
    goimports -w .

# Run linter
lint:
    golangci-lint run

# Run all tests
test:
    go test -v -race ./...

# Build the core backend
build-core:
    go build -o build/core cmd/alloy-core/main.go

# Run the insecure test script
test-smoke:
    chmod +x tests/test_smoke.sh
    ./tests/test_smoke.sh

# Run the UDS test script
test-uds:
    chmod +x tests/test_uds.sh
    ./tests/test_uds.sh

# Build the CLI
build-cli:
    go build -o build/frontend cmd/alloy-cli/main.go

# Build WASM plugins
build-wasm:
    ./build_plugins.sh

# Build everything
build-all: build-core build-cli build-wasm

# Run everything (format, lint, test, build)
all: fmt lint test build-all
