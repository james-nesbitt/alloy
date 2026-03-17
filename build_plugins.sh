#!/bin/bash
set -e

# Core and Frontend are handled by the Task Runner (justfile)
# This script handles WASM plugins specifically.

mkdir -p build/wasm

# Check for tinygo
if command -v tinygo >/dev/null 2>&1; then
    COMPILER="tinygo build -target=wasi"
else
    echo "Tinygo not found, using 'go build' with GOOS=wasip1 GOARCH=wasm (might lack WASI features)..."
    # We use 'env' to make sure the Go environment variables are correctly assigned
    COMPILER="env GOOS=wasip1 GOARCH=wasm go build"
fi

# Build all WASM plugins in plugins/wasm/ subdirectories
for plugin_dir in plugins/wasm/*; do
    if [ -d "$plugin_dir" ]; then
        plugin_name=$(basename "$plugin_dir")
        
        # Determine output name
        case "$plugin_name" in
            "chat-manager")
                target_name="chat"
                ;;
            "buffer-manager")
                target_name="buffer"
                ;;
            "ai-agent")
                target_name="ai"
                ;;
            *)
                target_name="$plugin_name"
                ;;
        esac
        
        echo "Building WASM: $plugin_name -> build/wasm/$target_name.wasm"
        # Compile directly to the build folder
        $COMPILER -o "build/wasm/$target_name.wasm" "$plugin_dir/main.go"
    fi
done
