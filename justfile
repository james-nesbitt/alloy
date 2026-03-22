# Alloy - Standard Project Build System
set shell := ["bash", "-c"]

# Project Configuration
PROJECT_ROOT := invocation_directory()
BUILD_DIR    := PROJECT_ROOT + "/build"
WASM_BUILD   := BUILD_DIR + "/wasm"
BIN_BUILD    := BUILD_DIR + "/bin"
PLUGINS_SRC  := PROJECT_ROOT + "/plugins/wasm"

# Default action
default: help

# Show help for available tasks
help:
    @just --list

# --- CLEANUP ---

# Remove build artifacts and temporary files
clean:
    rm -rf {{BUILD_DIR}}
    rm -rf .tmp-bin
    find . -name "wit" -type d -path "*/plugins/wasm/*" -exec rm -rf {} +
    @echo "Cleanup complete."

# --- PROJECT SETUP & CODE GENERATION ---

# Generate WIT bindings for both Go (guest) and Rust (host)
generate:
    @echo ">> Generating WIT bindings..."
    mkdir -p pkg/wasm/bindings/host/wit-rust pkg/wasm/bindings/guest
    wit-bindgen rust --out-dir pkg/wasm/bindings/host/wit-rust wit/alloy.wit
    wit-bindgen tiny-go --out-dir pkg/wasm/bindings/guest wit/alloy.wit
    # Initialize the guest bindings go.mod
    @if [ ! -f pkg/wasm/bindings/guest/go.mod ]; then \
        cd pkg/wasm/bindings/guest && go mod init github.com/jnesbitt/alloy-go/pkg/wasm/bindings/guest && cd -; \
    fi
    @echo ">> WIT bindings generated."

# Fix and standardize all project modules (replace directives, workspace)
setup-dev:
    @chmod +x scripts/fix-project-modules.sh
    @./scripts/fix-project-modules.sh

# --- BINARY BUILDS ---

# Build Alloy Core 
build-core:
    @echo ">> Building Alloy Core..."
    mkdir -p {{BIN_BUILD}}
    go build -o {{BIN_BUILD}}/alloy-core ./cmd/alloy-core

# Build Alloy TUI
build-tui:
    @echo ">> Building TUI Frontend..."
    mkdir -p {{BIN_BUILD}}
    go build -o {{BIN_BUILD}}/alloy-tui ./cmd/alloy-tui

# Build Alloy GUI (Native)
build-gui:
    @echo ">> Building Gio GUI..."
    mkdir -p {{BIN_BUILD}}
    go build -tags gui -o {{BIN_BUILD}}/alloy-gui ./cmd/alloy-gui-gio

# Build Alloy CLI
build-cli:
    @echo ">> Building Alloy CLI Tool..."
    mkdir -p {{BIN_BUILD}}
    go build -o {{BIN_BUILD}}/alloy-cli ./cmd/alloy-cli

# Build all binaries
build-binaries: build-core build-tui build-gui build-cli

# --- WASM PLUGIN BUILDS ---

# Build a single WASM plugin (e.g. just build-plugin health)
build-plugin name: generate
    @echo ">> Building WASM plugin: {{name}}..."
    mkdir -p {{WASM_BUILD}}
    @chmod +x scripts/build-plugin.sh
    @./scripts/build-plugin.sh {{name}}

# Build all WIT-based WASM plugins
build-plugins: generate
    @echo ">> Building all WASM plugins..."
    mkdir -p {{WASM_BUILD}}
    @./scripts/build-wasm-all.sh
    @echo ">> WASM plugins built successfully."

# Build everything
all: build-plugins build-binaries

# --- EXECUTION ---

# Run Alloy Core
run-core *args: build-core
    {{BIN_BUILD}}/alloy-core --debug {{args}}

# Run TUI Frontend
run-tui *args: build-tui
    {{BIN_BUILD}}/alloy-tui {{args}}

# Run Native GUI
run-gui *args: build-gui
    {{BIN_BUILD}}/alloy-gui {{args}}

# --- TESTING ---

# Run comprehensive test suite
test:
    @echo ">> Running unit tests..."
    go test -v ./pkg/...
    @echo ">> Running integration tests..."
    go test -v ./tests/...

# Run WASM specific tests
test-wasm:
    @echo ">> Running WASM/WIT implementation tests..."
    go test -v -run "TestWIT" ./pkg/wasm/... ./pkg/kernel/...
