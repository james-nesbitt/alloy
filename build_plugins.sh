#!/bin/bash
set -e

# Use TinyGo if available, fallback to Go
if command -v tinygo >/dev/null 2>&1; then
    echo "Using TinyGo for plugin builds..."
    # Build with wasip1 target and tasks scheduler
    COMPILER="tinygo build -target=wasip1 -scheduler=tasks"
else
    echo "Using 'go build' with GOOS=wasip1 GOARCH=wasm..."
    COMPILER="env GOOS=wasip1 GOARCH=wasm go build"
fi

# Build all WASM plugins in plugins/wasm/ subdirectories
for plugin_dir in plugins/wasm/*; do
    if [ -d "$plugin_dir" ]; then
        plugin_name=$(basename "$plugin_dir")
        
        # Determine output name
        case "$plugin_name" in
            "plugin-chat")
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
        # Compile the entire package directory
        $COMPILER -o "build/wasm/$target_name.wasm" "$plugin_dir"
    fi
done
