#!/bin/bash
set -euo pipefail

# This script standardizes all project modules (Go Workspace and replaces) 
# for consistent build/dev experience using the new standardized pkg/wasm

PROJECT_ROOT=$(git rev-parse --show-toplevel)
BUILD_DIR="$PROJECT_ROOT/build"
GEN_DIR="$BUILD_DIR/gen"
BINDINGS_DIR="$GEN_DIR/bindings/guest"
SDK_DIR="$PROJECT_ROOT/pkg/wasm/guest"

BINDINGS_MOD="github.com/james-nesbitt/alloy/build/gen/bindings/guest"
SDK_MOD="github.com/james-nesbitt/alloy/pkg/wasm/guest"

echo ">> Standardizing Root go.mod..."
cd "$PROJECT_ROOT"
# Clean up root go.mod: remove any old wasm relative replaces
sed -i '/bindings/d' go.mod
sed -i '/pkg\/wasm/d' go.mod
# Re-add project local replaces for internal modules
go mod edit -replace "$BINDINGS_MOD"=./build/gen/bindings/guest
go mod edit -replace "$SDK_MOD"=./pkg/wasm/guest
go mod tidy || true

echo ">> Fixing Guest Bindings go.mod..."
mkdir -p "$BINDINGS_DIR"
cd "$BINDINGS_DIR"
if [ ! -f go.mod ]; then
    go mod init "$BINDINGS_MOD"
fi
sed -i "s|^module .*|module $BINDINGS_MOD|" go.mod
sed -i 's|^go .*|go 1.25.8|' go.mod
go mod tidy || true

echo ">> Fixing Guest SDK go.mod..."
cd "$SDK_DIR"
if [ ! -f go.mod ]; then
    go mod init "$SDK_MOD"
fi
sed -i "s|^module .*|module $SDK_MOD|" go.mod
sed -i 's|^go .*|go 1.25.8|' go.mod
# Link SDK to bindings in build dir
go mod edit -replace "$BINDINGS_MOD"=../../../build/gen/bindings/guest
go mod tidy || true

echo ">> Fixing all WASM Plugins..."
find "$PROJECT_ROOT/plugins/wasm" -maxdepth 1 -mindepth 1 -type d | while read -r plugin_dir; do
    plugin_name=$(basename "$plugin_dir")
    echo "  - Standardizing $plugin_name..."
    cd "$plugin_dir"
    if [ ! -f go.mod ]; then
        go mod init "github.com/james-nesbitt/alloy/plugins/wasm/$plugin_name"
    fi
    # Clean old replaces
    sed -i '/bindings/d' go.mod
    sed -i '/pkg\/wasm/d' go.mod
    
    # Add new specific replaces
    go mod edit -replace "$BINDINGS_MOD"=../../../build/gen/bindings/guest
    go mod edit -replace "$SDK_MOD"=../../../pkg/wasm/guest
    
    # Normalize version
    sed -i 's|^go .*|go 1.25.8|' go.mod
    
    go mod tidy || echo "  >> Warning: tidy failed for $plugin_name"
done

echo ">> Cleanup and Regenerate go.work..."
cd "$PROJECT_ROOT"
rm -f go.work
go work init .
go work use ./build/gen/bindings/guest
go work use ./pkg/wasm/guest
find ./plugins/wasm -maxdepth 1 -mindepth 1 -type d -exec go work use {} +

echo ">> Project modules standardized for new pkg/wasm path."
