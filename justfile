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

# Build the WIT-based core backend
build-core-wit:
    go build -o build/core-wit ./cmd/alloy-core-wit

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

# Run the WIT-based Alloy Core
run-core-wit: build-core-wit build-wasm
    mkdir -p plugins-bin
    cp build/wasm/*.wasm plugins-bin/
    ./build/core-wit --debug

# Run Alloy Core with WIT example plugin (future)
run-wit-example: build-core build-wit-example
    mkdir -p plugins-bin
    cp build/wasm/wit-example.wasm plugins-bin/
    @echo "WIT runtime not yet implemented - this will be available in a future update"

# Install wit-bindgen tool
install-wit-bindgen:
    cargo install wit-bindgen-cli --version 0.24.0

# Generate WIT bindings for WASM plugins
generate-wit-bindings:
    mkdir -p pkg/wasm/bindings/host/wit-rust pkg/wasm/bindings/guest
    wit-bindgen rust --out-dir pkg/wasm/bindings/host/wit-rust wit/alloy.wit
    wit-bindgen tiny-go --out-dir pkg/wasm/bindings/guest wit/alloy.wit
    
    # Create go.mod files
    echo 'module github.com/jnesbitt/alloy-go/pkg/wasm/bindings/host\n\ngo 1.25' > pkg/wasm/bindings/host/go.mod
    echo 'module github.com/jnesbitt/alloy-go/pkg/wasm/bindings/guest\n\ngo 1.25' > pkg/wasm/bindings/guest/go.mod
    
    @echo "WIT bindings generated successfully!"

# Build all WASM plugins with WIT bindings
build-wasm:
    #!/usr/bin/env bash
    mkdir -p build/wasm
    mkdir -p .tmp-bin
    cp wasm-opt-shim.sh .tmp-bin/wasm-opt
    chmod +x .tmp-bin/wasm-opt
    TMP_BIN=$(realpath .tmp-bin)

    # Generate WIT bindings
    just generate-wit-bindings

    # Build plugins with WIT support
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
            echo "Building WASM with WIT: $plugin_name -> build/wasm/$target_name.wasm"
            
            # Copy WIT bindings to the plugin directory
            mkdir -p "$plugin_dir/wit"
            cp -r pkg/wasm2/bindings/guest/* "$plugin_dir/wit/"
            
            # Build with TinyGo for better WASM support
            (cd "$plugin_dir" && GOOS=wasip1 GOARCH=wasm tinygo build -target=wasi -o "../../../build/wasm/$target_name.wasm" .)
        fi
    done
    rm -rf .tmp-bin

# Build the WIT example plugin
build-wit-example: generate-wit-bindings
    mkdir -p build/wasm
    echo "Building WIT example plugin..."
    mkdir -p examples/wit-plugin/wit
    cp -r pkg/wasm2/bindings/guest/* examples/wit-plugin/wit/
    (cd examples/wit-plugin && GOOS=wasip1 GOARCH=wasm tinygo build -o ../../build/wasm/wit-example.wasm -target=wasi .)
    @echo "WIT example plugin built: build/wasm/wit-example.wasm"

# Build the WIT chat plugin
build-wit-chat: generate-wit-bindings
    mkdir -p build/wasm
    echo "Building WIT chat plugin..."
    mkdir -p examples/wit-chat-plugin/wit
    cp -r pkg/wasm2/bindings/guest/* examples/wit-chat-plugin/wit/
    (cd examples/wit-chat-plugin && GOOS=wasip1 GOARCH=wasm tinygo build -o ../../build/wasm/wit-chat.wasm -target=wasi .)
    @echo "WIT chat plugin built: build/wasm/wit-chat.wasm"

# Build the test WASM module
build-test-wasm: generate-wit-bindings
    mkdir -p build/wasm
    echo "Building test WASM module..."
    mkdir -p tests/wasm2/test_wasm/wit
    cp -r pkg/wasm2/bindings/guest/* tests/wasm2/test_wasm/wit/
    (cd tests/wasm2/test_wasm && GOOS=wasip1 GOARCH=wasm tinygo build -o ../../../build/wasm/test_wasm.wasm -target=wasi .)
    @echo "Test WASM module built: build/wasm/test_wasm.wasm"

# Build WASM plugins with wit-bindgen support (future)
build-wasm-wit: generate-wit-bindings build-wit-example
    @echo "Building WASM plugins with WIT support - implementation coming soon"

# Alias for building plugins
build-plugins: build-wasm

# Run everything (format, lint, test, build)
all: fmt lint test build-all
