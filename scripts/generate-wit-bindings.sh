#!/bin/bash
set -euo pipefail

# Create directories for generated code
mkdir -p pkg/wasm/bindings/host
mkdir -p pkg/wasm/bindings/guest

# Generate host bindings (Rust for now - we'll use this as a bridge)
echo "Generating host bindings (Rust)..."
wit-bindgen rust --out-dir pkg/wasm/bindings/host/wit-rust wit/alloy.wit

# Generate guest bindings (TinyGo)
echo "Generating guest bindings (TinyGo)..."
wit-bindgen tiny-go --out-dir pkg/wasm/bindings/guest wit/alloy.wit

# Create simple go.mod files for the bindings
echo "Creating go.mod files..."
cat > pkg/wasm/bindings/host/go.mod <<EOF
module github.com/jnesbitt/alloy-go/pkg/wasm/bindings/host

go 1.25
EOF

cat > pkg/wasm/bindings/guest/go.mod <<EOF
module github.com/jnesbitt/alloy-go/pkg/wasm/bindings/guest

go 1.25
EOF

echo "WIT bindings generated successfully!
Note: For Go host bindings, we'll need to use a different approach or wait for official Go support in wit-bindgen."