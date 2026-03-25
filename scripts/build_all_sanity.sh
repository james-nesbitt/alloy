#!/bin/bash
set -e

echo "--- BUILDING ALLOY COMPONENTS ---"

# 1. Build Kernel
echo "Building Core Kernel..."
go build -o build/alloy-core ./cmd/alloy-core

# 2. Build TUI
echo "Building TUI Frontend..."
go build -o build/alloy-tui ./cmd/alloy-tui

# 3. Build GUI (Gio)
echo "Building GUI Frontend..."
go build -tags gui -o build/alloy-gui ./cmd/alloy-gui-gio

# 4. Build Web Frontend
echo "Building Web Frontend (WASM)..."
GOOS=js GOARCH=wasm go build -o cmd/alloy-web/static/wasm/frontend.wasm ./cmd/alloy-web/internal/bridge/wasm_main.go
echo "Building Web Host Proxy..."
go build -o build/alloy-web ./cmd/alloy-web

# 5. Build Key Plugins
echo "Building Project Manager Plugin (WASM)..."
./build-plugin.sh plugins/wasm/project build/plugins/project.wasm

echo "--- ALL COMPONENTS BUILT SUCCESSFULLY ---"
