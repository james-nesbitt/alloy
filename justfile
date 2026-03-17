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
    go build -o bin/alloy-core cmd/alloy-core/main.go

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
    go build -o bin/alloy-cli cmd/alloy-cli/main.go

# Build everything
build-all: build-core build-cli

# Run everything (format, lint, test, build)
all: fmt lint test build-all
