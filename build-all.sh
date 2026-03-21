#!/bin/bash
set -euo pipefail

# Generate WIT bindings
just generate-wit-bindings

# Create build directory
mkdir -p build/wasm

# Build each plugin
for plugin_dir in plugins/wasm/*; do
    if [ -d "$plugin_dir" ]; then
        plugin_name=$(basename "$plugin_dir")
        echo "Building plugin: $plugin_name"
        
        # Copy WIT bindings to the plugin directory
        mkdir -p "$plugin_dir/wit"
        cp -r pkg/wasm2/bindings/guest/* "$plugin_dir/wit/"
        
        # Build with TinyGo
        if [ -f "$plugin_dir/main_wit.go" ]; then
            (cd "$plugin_dir" && GOOS=wasip1 GOARCH=wasm tinygo build -target=wasi -o "../../../build/wasm/$plugin_name.wasm" main_wit.go)
        else
            (cd "$plugin_dir" && GOOS=wasip1 GOARCH=wasm tinygo build -target=wasi -o "../../../build/wasm/$plugin_name.wasm" .)
        fi
        
        echo "Built: build/wasm/$plugin_name.wasm"
    fi
done

echo "All plugins built successfully!"