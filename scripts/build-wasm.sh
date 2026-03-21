#!/bin/bash
set -euo pipefail

# Create build directories
mkdir -p build/wasm
mkdir -p .tmp-bin
cp wasm-opt-shim.sh .tmp-bin/wasm-opt 2>/dev/null || true
chmod +x .tmp-bin/wasm-opt
TMP_BIN=$(realpath .tmp-bin)

# Generate WIT bindings
scripts/build-wit.sh

# Build plugins with WIT support
for plugin_dir in plugins/wasm/*; do
    if [ -d "$plugin_dir" ]; then
        plugin_name=$(basename "$plugin_dir")
        echo "Building WASM plugin: $plugin_name -> build/wasm/$plugin_name.wasm"
        
        # Copy WIT bindings to the plugin directory
        mkdir -p "$plugin_dir/wit"
        cp -r pkg/wasm2/bindings/guest/* "$plugin_dir/wit/"
        
        # Build with TinyGo for better WASM support
        if [ -f "$plugin_dir/main_wit.go" ]; then
            (cd "$plugin_dir" && GOOS=wasip1 GOARCH=wasm tinygo build -target=wasi -o "../../../build/wasm/$plugin_name.wasm" main_wit.go)
        else
            (cd "$plugin_dir" && GOOS=wasip1 GOARCH=wasm tinygo build -target=wasi -o "../../../build/wasm/$plugin_name.wasm" .)
        fi
    fi
done

rm -rf .tmp-bin
echo "WASM plugins built successfully!"