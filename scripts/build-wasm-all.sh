#!/bin/bash
set -euo pipefail

# This script builds all WIT-based WASM plugins in the plugins/wasm directory
PROJECT_ROOT=$(git rev-parse --show-toplevel)
PLUGINS_DIR="$PROJECT_ROOT/plugins/wasm"

echo ">> Beginning All-Plugin Build..."

for dir in "$PLUGINS_DIR"/*; do
    if [ -d "$dir" ]; then
        plugin_name=$(basename "$dir")
        echo ">> Dispatching build for: $plugin_name"
        just build-plugin "$plugin_name"
    fi
done

echo ">> All WASM plugins built."
