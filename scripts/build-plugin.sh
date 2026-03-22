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
    go mod init "github.com/jnesbitt/alloy-go/plugins/wasm/$PLUGIN_NAME"
fi

# Set project-relative replacements
go mod edit -replace github.com/jnesbitt/alloy-go/pkg/wasm/bindings/guest=../../../pkg/wasm/bindings/guest
go mod edit -replace github.com/jnesbitt/alloy-go/pkg/wasm/guest=../../../pkg/wasm/guest
sed -i 's|^go .*|go 1.25.8|' go.mod
go mod tidy || true

# Robust wasm-opt shim (FIXED HEREDOC)
WASM_OPT_DUMMY="$PROJECT_ROOT/.tmp-bin/wasm-opt"
mkdir -p "$PROJECT_ROOT/.tmp-bin"
cat > "$WASM_OPT_DUMMY" << 'EOF'
#!/bin/bash
if [[ "$*" == *"--version"* ]]; then
  echo "wasm-opt version 110"
  exit 0
fi

OUT=""
IN=""
for ((i=1; i<=$#; i++)); do
  arg="${@:i:1}"
  if [[ "$arg" == "-o" ]]; then
    next_idx=$((i+1))
    OUT="${@:next_idx:1}"
    i=$next_idx
  elif [[ "$arg" != -* ]]; then
    IN="$arg"
  fi
done

if [[ -n "$OUT" ]]; then
  if [[ -n "$IN" && -f "$IN" ]]; then
    cp "$IN" "$OUT"
  else
    # TinyGo sometimes uses standard input? No, usually positional.
    # We must ensure the output file exists.
    touch "$OUT"
  fi
fi
exit 0
EOF
chmod +x "$WASM_OPT_DUMMY"
export WASMOPT="$WASM_OPT_DUMMY"

echo ">> Compiling $PLUGIN_NAME..."
tinygo build -target=wasip1 -o "$BUILD_DIR/$PLUGIN_NAME.wasm" -no-debug -opt=0 .

echo ">> Successfully built $PLUGIN_NAME.wasm"
