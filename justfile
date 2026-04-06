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
LOGS_DIR     := BUILD_DIR + "/logs"
DATA_DIR     := BUILD_DIR + "/data"

# Build configuration
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

# --- CLEANUP ---

# Format all Go source code
fmt:
    @echo ">> Formatting Go source code..."
    go fmt ./...

# Remove build artifacts and temporary files
clean:
    @echo ">> Cleaning up build directory..."
    rm -rf {{BUILD_DIR}}
    @echo ">> Cleanup complete."

# --- PROJECT SETUP & CODE GENERATION ---

# Check project dependencies (Go, TinyGo, wit-bindgen)
check-deps:
    @command -v go >/dev/null 2>&1 || { echo >&2 "Go is required. Install from: https://go.dev"; exit 1; }
    @command -v tinygo >/dev/null 2>&1 || { echo >&2 "TinyGo is required. Install from: https://tinygo.org"; exit 1; }
    @command -v wit-bindgen >/dev/null 2>&1 || { echo >&2 "wit-bindgen is required (v0.17.0 recommended). Install: 'cargo install wit-bindgen-cli --version 0.17.0'"; exit 1; }
    @if ! wit-bindgen --version | grep -q "0.17.0"; then \
        echo "WARNING: Detected wit-bindgen $(wit-bindgen --version). Version 0.17.0 is highly recommended for TinyGo compatibility."; \
    fi

# Ensure mock wasm-opt exists (TinyGo dependency)
setup-wasm-opt:
    @mkdir -p {{INTERNAL_BIN}}
    @echo '#!/bin/bash' > {{INTERNAL_BIN}}/wasm-opt
    @echo 'if [[ "$1" == "--version" ]]; then echo "wasm-opt version 121"; exit 0; fi' >> {{INTERNAL_BIN}}/wasm-opt
    @echo '# Mock wasm-opt to skip optimization steps if Binaryen is missing' >> {{INTERNAL_BIN}}/wasm-opt
    @echo 'while [[ $# -gt 0 ]]; do' >> {{INTERNAL_BIN}}/wasm-opt
    @echo '  case $1 in' >> {{INTERNAL_BIN}}/wasm-opt
    @echo '    --output) OUTPUT="$2"; shift; shift ;;' >> {{INTERNAL_BIN}}/wasm-opt
    @echo '    *) INPUT="$1"; shift ;;' >> {{INTERNAL_BIN}}/wasm-opt
    @echo '  esac' >> {{INTERNAL_BIN}}/wasm-opt
    @echo 'done' >> {{INTERNAL_BIN}}/wasm-opt
    @echo 'if [ ! -z "$OUTPUT" ]; then cp "$INPUT" "$OUTPUT"; fi' >> {{INTERNAL_BIN}}/wasm-opt
    @chmod +x {{INTERNAL_BIN}}/wasm-opt

# Generate WIT bindings for both Go (guest) and Rust (host)
generate: check-deps
    @echo ">> Generating WIT bindings into {{BINDINGS_DIR}}..."
    mkdir -p {{BINDINGS_DIR}}/host/wit-rust {{BINDINGS_DIR}}/guest
    wit-bindgen rust --out-dir {{BINDINGS_DIR}}/host/wit-rust wit/alloy.wit
    wit-bindgen tiny-go --out-dir {{BINDINGS_DIR}}/guest wit/alloy.wit
    @if [ ! -f {{BINDINGS_DIR}}/guest/go.mod ]; then \
        cd {{BINDINGS_DIR}}/guest && go mod init github.com/james-nesbitt/alloy/build/gen/bindings/guest && cd -; \
    fi
    @echo ">> WIT bindings generated."

# Fix and standardize all project modules (replace directives, workspace)
setup-dev: generate setup-wasm-opt
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
    @echo ">> Building Web Frontend (WASM) into {{BUILD_DIR}}/web/static/wasm/..."
    @mkdir -p {{BUILD_DIR}}/web/static/wasm
    @GOOS=js GOARCH=wasm go build -o {{BUILD_DIR}}/web/static/wasm/frontend.wasm ./cmd/alloy-web/internal/bridge/wasm_main.go
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

# --- EXECUTION & TESTING ---

# Run comprehensive test suite (output test logs to build/logs)
test:
    @mkdir -p {{LOGS_DIR}}
    @echo ">> Running unit tests (logs to {{LOGS_DIR}}/unit_tests.log)..."
    go test -v ./pkg/... | tee {{LOGS_DIR}}/unit_tests.log
    @echo ">> Running integration tests (logs to {{LOGS_DIR}}/integration_tests.log)..."
    go test -v ./tests/... | tee {{LOGS_DIR}}/integration_tests.log

# Run all tests (Unit, Integration, and Web)
test-all: test-web test
    @echo ">> All tests completed successfully."

# JS/WASM bridge UX tests
test-web: install-web-deps
    @mkdir -p {{LOGS_DIR}}
    @echo ">> Running Go tests for web proxy (logs to {{LOGS_DIR}}/web_tests.log)..."
    go test -v ./cmd/alloy-web | tee {{LOGS_DIR}}/web_tests.log
    @echo ">> Running JS/WASM Bridge UX tests..."
    cd cmd/alloy-web && npm test

install-web-deps:
    @echo ">> Installing web dependencies..."
    cd cmd/alloy-web && npm install

# --- PHASE 13 DEVELOPMENT ---

# Start a dedicated core and open the current folder as an Alloy Base
develop: build-all
	#!/usr/bin/env bash
	echo ">> Launching Alloy Development Environment..."
	rm -f {{BUILD_DIR}}/alloy-dev.sock
	mkdir -p {{BUILD_DIR}}/data-dev
	{{LIBEXEC_DIR}}/alloy-core --insecure --listen unix://{{BUILD_DIR}}/alloy-dev.sock --data-dir {{BUILD_DIR}}/data-dev --debug > {{BUILD_DIR}}/core.log 2>&1 &
	CORE_PID=$!
	echo $CORE_PID > {{BUILD_DIR}}/core.pid
	sleep 2
	if ! {{BIN_DIR}}/alloy open --insecure --socket unix://{{BUILD_DIR}}/alloy-dev.sock .; then
		echo "Failed to open project. Check {{BUILD_DIR}}/core.log for details."
		kill $CORE_PID
		exit 1
	fi
	{{BIN_DIR}}/alloy tui --insecure --socket unix://{{BUILD_DIR}}/alloy-dev.sock

	kill $CORE_PID
	rm {{BUILD_DIR}}/core.pid
