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
    go test -v -race -timeout 300s ./...

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
    go build -o build/cli cmd/alloy-cli/main.go

# Build the TUI
build-tui:
    go build -o build/tui cmd/alloy-tui/main.go

# Build the Gio GUI
build-gui-gio:
	go build -tags gui -o build/gui-gio cmd/alloy-gui-gio/main.go

# Build the Pure Go Wayland Native GUI (X11-free)
build-gui-wayland:
	go build -o build/gui-wayland cmd/alloy-gui-wayland-native/main.go

# Build everything
build-all: build-core build-cli build-tui build-gui build-wasm

# Run the Alloy Core backend
run-core *args: build-core
    ./build/core {{args}}

# Run the Alloy TUI frontend
run-tui *args: build-tui
    ./build/tui {{args}}

# Run the Alloy GUI frontend
run-gui *args: build-gui
    ./build/gui-gio {{args}}

# Run Alloy Core with debug enabled
debug-core: build-core
    ./build/core --debug

# Run Alloy Core with health plugin provisioned
run-health: build-core build-wasm
    mkdir -p plugins-bin
    cp build/wasm/health-wasm.wasm plugins-bin/
    ./build/core --provision provision.json --debug

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

# Alias for building plugins
build-plugins: build-wasm

# Run everything (format, lint, test, build)
all: fmt lint test build-all
