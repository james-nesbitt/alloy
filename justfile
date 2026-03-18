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
    go test -v -race -timeout 60s ./...

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

# Build everything
build-all: build-core build-cli build-wasm

# Build all WASM plugins
build-wasm:
    #!/usr/bin/env bash
    mkdir -p build/wasm
    for plugin_dir in plugins/wasm/*; do
        if [ -d "$plugin_dir" ]; then
            plugin_name=$(basename "$plugin_dir")
            case "$plugin_name" in
                "chat-manager") target_name="chat" ;;
                "buffer-manager") target_name="buffer" ;;
                "ai-agent") target_name="ai" ;;
                *) target_name="$plugin_name" ;;
            esac
            echo "Building WASM: $plugin_name -> build/wasm/$target_name.wasm"
            GOOS=wasip1 GOARCH=wasm go build -o "build/wasm/$target_name.wasm" "$plugin_dir/main.go"
        fi
    done

# Run everything (format, lint, test, build)
all: fmt lint test build-all
