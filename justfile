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

# Clean up build artifacts and any leftover processes
clean: kill-alloy
    rm -rf build/

# Kill any leftover alloy processes safely
kill-alloy:
    @-pkill -x core || true
    @-pkill -x tui || true
    @-pkill -x gui-gio || true
    @-pkill -x gui-wayland || true
    @-pkill -x cli || true

# Run linter
lint:
    golangci-lint run

# Run all tests
test: kill-alloy build-core build-wasm
    go test -v -timeout 120s ./...

# Build the core backend
build-core:
    go build -o build/core ./cmd/alloy-core

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
    go build -o build/cli ./cmd/alloy-cli

# Build the TUI
build-tui:
    go build -o build/tui ./cmd/alloy-tui

# Build the Gio GUI
build-gui-gio:
	go build -tags gui -o build/gui-gio ./cmd/alloy-gui-gio

# Build the Pure Go Wayland Native GUI (X11-free, Experimental)
build-gui-wayland:
	go build -tags experimental_wayland -o build/gui-wayland ./cmd/alloy-gui-wayland-native

# Build everything
build-all: build-core build-cli build-tui build-gui-gio build-gui-wayland build-wasm

# Run the Alloy Core backend
run-core *args: build-core
    ./build/core {{args}}

# Run the Alloy TUI frontend
run-tui *args: build-tui
    ./build/tui {{args}}

# Run the Alloy Gio GUI frontend
run-gui-gio *args: build-gui-gio
    ./build/gui-gio {{args}}

# Run the Alloy Wayland Native GUI frontend
run-gui-wayland *args: build-gui-wayland
    ./build/gui-wayland {{args}}

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
    mkdir -p .tmp-bin
    cp wasm-opt-shim.sh .tmp-bin/wasm-opt
    chmod +x .tmp-bin/wasm-opt
    TMP_BIN=$(realpath .tmp-bin)

    for plugin_dir in plugins/wasm/*; do
        if [ -d "$plugin_dir" ]; then
            plugin_name=$(basename "$plugin_dir")
            case "$plugin_name" in
                "plugin-chat") target_name="chat" ;;
                "buffer-manager") target_name="buffer" ;;
                "ai-agent") target_name="ai" ;;
                "health-wasm") target_name="health" ;;
                "plugin-project-manager") target_name="project" ;;
                *) target_name="$plugin_name" ;;
            esac
            echo "Building WASM: $plugin_name -> build/wasm/$target_name.wasm"
            # Use TinyGo if available, otherwise Standard Go wasip1
            if command -v tinygo >/dev/null 2>&1; then
                (cd "$plugin_dir" && PATH="$TMP_BIN:$PATH" tinygo build -target=wasip1 -o "../../../build/wasm/$target_name.wasm" .)
            else
                (cd "$plugin_dir" && GOOS=wasip1 GOARCH=wasm go build -o "../../../build/wasm/$target_name.wasm" .)
            fi
        fi
    done
    rm -rf .tmp-bin

# Alias for building plugins
build-plugins: build-wasm

# Run everything (format, lint, test, build)
all: fmt lint test build-all
