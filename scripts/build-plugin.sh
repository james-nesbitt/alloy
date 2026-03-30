#!/bin/bash
set -euo pipefail

# This script builds a single WASM plugin (WIT Standard)
PLUGIN_NAME="$1"
PROJECT_ROOT="$(git rev-parse --show-toplevel)"
PLUGIN_DIR="$PROJECT_ROOT/plugins/wasm/$PLUGIN_NAME"
# Allow override of output directory
if [ -z "${WASM_OUT:-}" ]; then
    WASM_OUT="$PROJECT_ROOT/build/dist/usr/lib/alloy/plugins"
fi
BUILD_DIR="$WASM_OUT"
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

# Provide mock wasm-opt or just use tinygo defaults
if [ ! -f "$PROJECT_ROOT/build/tmp/bin/wasm-opt" ]; then
    unset WASMOPT
    OPT_FLAG="-opt=0"
else
    export WASMOPT="$PROJECT_ROOT/build/tmp/bin/wasm-opt"
    OPT_FLAG="-opt=1"
fi

echo ">> Compiling $PLUGIN_NAME into $BUILD_DIR..."
tinygo build -target=wasip1 -o "$BUILD_DIR/$PLUGIN_NAME.wasm" -no-debug $OPT_FLAG .

echo ">> Successfully built $PLUGIN_NAME.wasm"
