# Alloy - Standard Project Build System
set shell := ["bash", "-c"]

# Project Configuration
PROJECT_ROOT := invocation_directory()
BUILD_DIR    := PROJECT_ROOT + "/build"

# Installation-style layout (Linux FHS/XDG style)
DIST_ROOT    := BUILD_DIR + "/dist"
BIN_DIR      := DIST_ROOT + "/usr/bin"
LIBEXEC_DIR  := DIST_ROOT + "/usr/libexec/alloy"
PLUGIN_DIR   := DIST_ROOT + "/usr/lib/alloy/plugins"
ETC_DIR      := DIST_ROOT + "/etc/alloy"

WASM_BUILD   := PLUGIN_DIR
BIN_BUILD    := BIN_DIR
GEN_DIR      := BUILD_DIR + "/gen"
INTERNAL_BIN := BUILD_DIR + "/tmp/bin"
BINDINGS_DIR := GEN_DIR + "/bindings"
PLUGINS_SRC  := PROJECT_ROOT + "/plugins/wasm"

# Default action
default: help

# Show help for available tasks
help:
    @just --list

# --- FLATPAK ---

# Build everything and then build a flatpak for the GUI
flatpak: build-all
    flatpak-builder --force-clean --user --install build/flatpak com.james_nesbitt.AlloyGui.yaml
    @echo "Alloy GUI Flatpak (com.james-nesbitt.AlloyGui) built and installed."

# --- CLEANUP ---

# Remove build artifacts and temporary files
clean:
    rm -rf {{BUILD_DIR}}
    @echo "Cleanup complete."

# --- PROJECT SETUP & CODE GENERATION ---

# Generate WIT bindings for both Go (guest) and Rust (host)
generate:
    @echo ">> Generating WIT bindings into {{BINDINGS_DIR}}..."
    mkdir -p {{BINDINGS_DIR}}/host/wit-rust {{BINDINGS_DIR}}/guest
    wit-bindgen rust --out-dir {{BINDINGS_DIR}}/host/wit-rust wit/alloy.wit
    wit-bindgen tiny-go --out-dir {{BINDINGS_DIR}}/guest wit/alloy.wit
    # Initialize the guest bindings go.mod
    @if [ ! -f {{BINDINGS_DIR}}/guest/go.mod ]; then \
        cd {{BINDINGS_DIR}}/guest && go mod init github.com/james-nesbitt/alloy/build/gen/bindings/guest && cd -; \
    fi
    @echo ">> WIT bindings generated."

# Fix and standardize all project modules (replace directives, workspace)
setup-dev: generate
    @chmod +x scripts/fix-project-modules.sh
    @./scripts/fix-project-modules.sh

# --- BINARY BUILDS ---

# Build Alloy Core 
build-core:
    @echo ">> Building Alloy Core into {{LIBEXEC_DIR}}..."
    mkdir -p {{LIBEXEC_DIR}}
    go build -o {{LIBEXEC_DIR}}/alloy-core ./cmd/alloy-core

# Build Alloy TUI
build-tui:
    @echo ">> Building TUI Frontend into {{BIN_DIR}}..."
    mkdir -p {{BIN_DIR}}
    go build -o {{BIN_DIR}}/alloy-tui ./cmd/alloy-tui

# Build Alloy GUI (Native)
build-gui:
    @echo ">> Building Gio GUI into {{BIN_DIR}}..."
    mkdir -p {{BIN_DIR}}
    go build -tags gui -o {{BIN_DIR}}/alloy-gui ./cmd/alloy-gui-gio

# Build Alloy Web (WASM + Host Proxy)
build-web:
    @echo ">> Building Web Frontend (WASM) into cmd/alloy-web/static/wasm/..."
    mkdir -p cmd/alloy-web/static/wasm
    GOOS=js GOARCH=wasm go build -o cmd/alloy-web/static/wasm/frontend.wasm ./cmd/alloy-web/internal/bridge/wasm_main.go
    @echo ">> Building Web Host Proxy into {{BIN_DIR}}..."
    mkdir -p {{BIN_DIR}}
    go build -o {{BIN_DIR}}/alloy-web ./cmd/alloy-web

# Build Alloy CLI
build-alloy:
    @echo ">> Building Alloy Tool into {{BIN_DIR}}..."
    mkdir -p {{BIN_DIR}}
    go build -o {{BIN_DIR}}/alloy ./cmd/alloy

# Build all binaries
build-binaries: build-core build-tui build-gui build-web build-alloy

# --- WASM PLUGIN BUILDS ---

# Build a single WASM plugin (e.g. just build-plugin health)
build-plugin name: generate
    @echo ">> Building WASM plugin: {{name}} into {{WASM_BUILD}}..."
    mkdir -p {{WASM_BUILD}}
    @chmod +x scripts/build-plugin.sh
    @./scripts/build-plugin.sh {{name}}

# Build all WIT-based WASM plugins
build-plugins: generate
	@echo ">> Building all WASM plugins into {{WASM_BUILD}}..."
	mkdir -p {{WASM_BUILD}}
	@chmod +x scripts/build-wasm-all.sh
	@./scripts/build-wasm-all.sh
	@echo ">> WASM plugins built successfully."

# Build everything
build-all: setup-dev build-plugins build-binaries pack-config
    @echo ">> Build complete. Optimized layout in {{DIST_ROOT}}"

# Package default configuration
pack-config:
    mkdir -p {{ETC_DIR}}
    @if [ -f provision.json ]; then cp provision.json {{ETC_DIR}}/provision.json; fi
    @if [ -f provision-wit.json ]; then cp provision-wit.json {{ETC_DIR}}/provision.json; fi

# --- EXECUTION ---

# Run Alloy Core
run-core *args: build-core
    {{LIBEXEC_DIR}}/alloy-core {{args}}

# Run TUI Frontend
run-tui *args: build-tui
    {{BIN_DIR}}/alloy-tui {{args}}

# Run Native GUI
run-gui *args: build-gui
    {{BIN_DIR}}/alloy-gui {{args}}

# Run Web Frontend (Host Proxy)
run-web *args: build-web
    {{BIN_DIR}}/alloy-web {{args}}

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

# Run unified sanity check (Kernel + Multiple simulated frontends)
test-unified: build-plugins
    @echo ">> Running Unified Sanity Check..."
    go test -v ./tests/sanity_check_test.go
