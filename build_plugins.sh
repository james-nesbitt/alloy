#!/bin/bash
set -e

# For this verification, we use standard 'go' with GOOS=js GOARCH=wasm if tinygo isn't available, 
# but wazero prefers tinygo for 'wasi' target.
# Since tinygo might not be on the machine, we'll try to use go build -o ... wasm
# HOWEVER, alloy-go expects a WASI-compliant module for things like FD usage or certain imports.
# Let's check for tinygo.

if command -v tinygo >/dev/null 2>&1; then
    echo "Found tinygo, building WASI modules..."
    tinygo build -o plugins/wasm/chat-manager/chat.wasm -target=wasi plugins/wasm/chat-manager/main.go
    tinygo build -o plugins/wasm/buffer-manager/buffer.wasm -target=wasi plugins/wasm/buffer-manager/main.go
else
    echo "Tinygo not found, falling back to go build (might not work perfectly with wazero WASI if imports mismatch)..."
    # wazero can often run GOOS=js GOARCH=wasm but the Host ABI we wrote uses unsafe.StringData (Go 1.20+)
    # and expect simple pointers.
    GOOS=wasip1 GOARCH=wasm go build -o plugins/wasm/chat-manager/chat.wasm plugins/wasm/chat-manager/main.go
    GOOS=wasip1 GOARCH=wasm go build -o plugins/wasm/buffer-manager/buffer.wasm plugins/wasm/buffer-manager/main.go
fi
