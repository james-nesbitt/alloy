#!/bin/bash
set -euo pipefail

# This script standardizes all project modules (Go Workspace and replaces) 
# for consistent build/dev experience using the new standardized pkg/wasm

PROJECT_ROOT=$(git rev-parse --show-toplevel)
BINDINGS_MOD="github.com/jnesbitt/alloy-go/pkg/wasm/bindings/guest"
SDK_MOD="github.com/jnesbitt/alloy-go/pkg/wasm/guest"

echo ">> Standardizing Root go.mod..."
cd "$PROJECT_ROOT"
# Clean up root go.mod: remove any old wasm2 relative replaces
sed -i '/pkg\/wasm/d' go.mod
# Re-add project local replaces for internal modules
go mod edit -replace "$BINDINGS_MOD"=./pkg/wasm/bindings/guest
go mod edit -replace "$SDK_MOD"=./pkg/wasm/guest
go mod tidy || true

echo ">> Fixing Guest Bindings go.mod..."
BINDINGS_DIR="$PROJECT_ROOT/pkg/wasm/bindings/guest"
mkdir -p "$BINDINGS_DIR"
cd "$BINDINGS_DIR"
if [ ! -f go.mod ]; then
    go mod init "$BINDINGS_MOD"
fi
sed -i "s|^module .*|module $BINDINGS_MOD|" go.mod
sed -i 's|^go .*|go 1.25.8|' go.mod
go mod tidy || true

echo ">> Fixing Guest SDK go.mod..."
SDK_DIR="$PROJECT_ROOT/pkg/wasm/guest"
mkdir -p "$SDK_DIR"
cd "$SDK_DIR"
if [ ! -f go.mod ]; then
    go mod init "$SDK_MOD"
fi
sed -i "s|^module .*|module $SDK_MOD|" go.mod
sed -i 's|^go .*|go 1.25.8|' go.mod
# Link SDK to bindings
go mod edit -replace "$BINDINGS_MOD"=../bindings/guest
go mod tidy || true

echo ">> Fixing all WASM Plugins..."
find "$PROJECT_ROOT/plugins/wasm" -maxdepth 1 -mindepth 1 -type d | while read -r plugin_dir; do
    plugin_name=$(basename "$plugin_dir")
    echo "  - Standardizing $plugin_name..."
    cd "$plugin_dir"
    if [ ! -f go.mod ]; then
        go mod init "github.com/jnesbitt/alloy-go/plugins/wasm/$plugin_name"
    fi
    # Clean old replaces
    sed -i '/pkg\/wasm/d' go.mod
    
    # Add new specific replaces
    go mod edit -replace "$BINDINGS_MOD"=../../../pkg/wasm/bindings/guest
    go mod edit -replace "$SDK_MOD"=../../../pkg/wasm/guest
    
    # Normalize version
    sed -i 's|^go .*|go 1.25.8|' go.mod
    
    go mod tidy || echo "  >> Warning: tidy failed for $plugin_name"
done

echo ">> Cleanup and Regenerate go.work..."
cd "$PROJECT_ROOT"
rm -f go.work
go work init .
go work use ./pkg/wasm/bindings/guest
go work use ./pkg/wasm/guest
find ./plugins/wasm -maxdepth 1 -mindepth 1 -type d -exec go work use {} +

echo ">> Project modules standardized for new pkg/wasm path."
