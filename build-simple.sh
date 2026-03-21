#!/bin/bash
set -euo pipefail

if [ $# -ne 1 ]; then
    echo "Usage: $0 <plugin-name>"
    exit 1
fi

plugin_name=$1
plugin_dir="plugins/wasm/$plugin_name"

if [ ! -d "$plugin_dir" ]; then
    echo "Plugin directory not found: $plugin_dir"
    exit 1
fi

echo "Building plugin: $plugin_name"
mkdir -p build/wasm

# Build with TinyGo
if [ -f "$plugin_dir/main_wit.go" ]; then
    (cd "$plugin_dir" && GOOS=wasip1 GOARCH=wasm tinygo build -no-debug -target=wasi -o "../../../build/wasm/$plugin_name.wasm" main_wit.go)
else
    (cd "$plugin_dir" && GOOS=wasip1 GOARCH=wasm tinygo build -no-debug -target=wasi -o "../../../build/wasm/$plugin_name.wasm" .)
fi

echo "Built: build/wasm/$plugin_name.wasm"