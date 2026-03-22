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

# Check if wasm-opt is available
REAL_WASMOPT="$PROJECT_ROOT/build/tmp/bin/wasm-opt"
OPT_FLAG="-opt=s"

if [ -f "$REAL_WASMOPT" ]; then
    export WASMOPT="$REAL_WASMOPT"
    echo ">> Using local wasm-opt: $REAL_WASMOPT"
elif command -v wasm-opt &> /dev/null; then
    echo ">> Using system wasm-opt"
else
    # TinyGo requires wasm-opt for non-zero opt levels on most targets
    echo ">> No wasm-opt found, disabling optimization"
    export WASMOPT=""
    OPT_FLAG="-opt=0"
fi

echo ">> Compiling $PLUGIN_NAME into $BUILD_DIR..."
tinygo build -target=wasip1 -o "$BUILD_DIR/$PLUGIN_NAME.wasm" -no-debug $OPT_FLAG .

echo ">> Successfully built $PLUGIN_NAME.wasm"
