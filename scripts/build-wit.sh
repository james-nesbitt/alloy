#!/bin/bash
set -euo pipefail

# Generate WIT bindings
mkdir -p pkg/wasm2/bindings/host/wit-rust pkg/wasm2/bindings/guest
wit-bindgen rust --out-dir pkg/wasm2/bindings/host/wit-rust wit/alloy.wit
wit-bindgen tiny-go --out-dir pkg/wasm2/bindings/guest wit/alloy.wit

# Create go.mod files
echo 'module github.com/jnesbitt/alloy-go/pkg/wasm2/bindings/host\n\ngo 1.25' > pkg/wasm2/bindings/host/go.mod
echo 'module github.com/jnesbitt/alloy-go/pkg/wasm2/bindings/guest\n\ngo 1.25' > pkg/wasm2/bindings/guest/go.mod

echo "WIT bindings generated successfully!"