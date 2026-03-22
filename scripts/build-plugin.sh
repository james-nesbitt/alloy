#!/bin/bash
set -euo pipefail

# This script builds a single WASM plugin (WIT Standard)
PLUGIN_NAME="$1"
PROJECT_ROOT="$(git rev-parse --show-toplevel)"
PLUGIN_DIR="$PROJECT_ROOT/plugins/wasm/$PLUGIN_NAME"
BUILD_DIR="$PROJECT_ROOT/build/wasm"

if [ ! -d "$PLUGIN_DIR" ]; then exit 1; fi
mkdir -p "$BUILD_DIR"
cd "$PLUGIN_DIR"

if [ ! -f "go.mod" ]; then
    go mod init "github.com/james-nesbitt/alloy/plugins/wasm/$PLUGIN_NAME"
fi

# Set project-relative replacements
go mod edit -replace github.com/james-nesbitt/alloy/build/gen/bindings/guest=../../../build/gen/bindings/guest
go mod edit -replace github.com/james-nesbitt/alloy/pkg/wasm/guest=../../../pkg/wasm/guest
sed -i 's|^go .*|go 1.25.8|' go.mod
go mod tidy || true

# Use the real wasm-opt we just downloaded (if present)
REAL_WASMOPT="$PROJECT_ROOT/.tmp-bin/wasm-opt"
if [ -f "$REAL_WASMOPT" ]; then
    export WASMOPT="$REAL_WASMOPT"
    OPT_FLAG="-opt=s" # Real optimizer can handle it!
else
    # Fallback to no optimization if binary not found
    export WASMOPT=""
    OPT_FLAG="-opt=0"
fi

echo ">> Compiling $PLUGIN_NAME..."
tinygo build -target=wasip1 -o "$BUILD_DIR/$PLUGIN_NAME.wasm" -no-debug $OPT_FLAG .

echo ">> Successfully built $PLUGIN_NAME.wasm"
