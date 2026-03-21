# Alloy WIT Build System

# Set shell
SHELL := "bash"

# Default target
default: help

# Show help
help:
	@echo "Alloy WIT Build System"
	@echo ""
	@echo "Targets:"
	@echo "  generate-wit-bindings - Generate WIT bindings"
	@echo "  build-wasm          - Build all WASM plugins"
	@echo "  build-plugin PLUGIN - Build specific plugin"
	@echo "  build-core-wit      - Build WIT-based core backend"
	@echo "  build-gui-tui       - Build TUI interface"
	@echo "  build-gui-web       - Build Web interface"
	@echo "  build-gui-all       - Build all GUIs"
	@echo "  build               - Build everything"
	@echo "  test-wit            - Run WIT tests"
	@echo "  test                - Run all tests"
	@echo "  clean               - Clean build artifacts"
	@echo "  fmt                 - Format code"

# Generate WIT bindings
generate-wit-bindings:
	@echo "Generating WIT bindings..."
	@mkdir -p pkg/wasm2/bindings/host/wit-rust pkg/wasm2/bindings/guest
	@wit-bindgen rust --out-dir pkg/wasm2/bindings/host/wit-rust wit/alloy.wit
	@wit-bindgen tiny-go --out-dir pkg/wasm2/bindings/guest wit/alloy.wit
	@echo 'module github.com/jnesbitt/alloy-go/pkg/wasm2/bindings/host\n\ngo 1.25' > pkg/wasm2/bindings/host/go.mod
	@echo 'module github.com/jnesbitt/alloy-go/pkg/wasm2/bindings/guest\n\ngo 1.25' > pkg/wasm2/bindings/guest/go.mod
	@echo "WIT bindings generated successfully!"

# Build all WASM plugins
build-wasm: generate-wit-bindings
	@echo "Building WASM plugins..."
	@mkdir -p build/wasm
	@for plugin in ai buffer chat health iam project secrets tasks; do \
		just build-plugin "$$plugin"; \
	done
	@echo "WASM plugins built successfully!"

# Build specific plugin
build-plugin plugin_name:
	@echo "Building plugin: {{plugin_name}}"
	@mkdir -p build/wasm
	@if [ -f "plugins/wasm/{{plugin_name}}/main_wit.go" ]; then \
		cd "plugins/wasm/{{plugin_name}}" && GOOS=wasip1 GOARCH=wasm tinygo build -no-debug -target=wasi -o "../../../build/wasm/{{plugin_name}}.wasm" main_wit.go; \
	else \
		cd "plugins/wasm/{{plugin_name}}" && GOOS=wasip1 GOARCH=wasm tinygo build -no-debug -target=wasi -o "../../../build/wasm/{{plugin_name}}.wasm" .; \
	fi
	@echo "Built: build/wasm/{{plugin_name}}.wasm"

# Build WIT-based core backend
build-core-wit:
	@echo "Building WIT-based core backend..."
	@mkdir -p build
	@go build -o build/core-wit ./cmd/alloy-core-wit
	@echo "Built: build/core-wit"

# Build TUI interface
build-gui-tui:
	@echo "Building TUI interface..."
	@mkdir -p build
	@go build -o build/alloy-tui ./cmd/alloy-tui
	@echo "Built: build/alloy-tui"

# Build Web interface
build-gui-web:
	@echo "Building Web interface..."
	@mkdir -p build
	@go build -o build/alloy-web ./cmd/alloy-web
	@echo "Built: build/alloy-web"

# Build all GUIs
build-gui-all: build-gui-tui build-gui-web
	@echo "All GUIs built successfully!"

# Build everything
build: build-wasm build-core-wit build-gui-all
	@echo "Build completed!"

# Run WIT tests
test-wit:
	@echo "Running WIT tests..."
	@cd pkg/wasm2/runtime && go test -v
	@cd pkg/wasm2 && go test -v
	@cd pkg/kernel && go test -v -run "TestWIT.*"
	@echo "WIT tests completed!"

# Run all tests
test: test-wit
	@echo "All tests completed!"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf build/
	@rm -rf .tmp-bin/
	@echo "Clean completed!"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@goimports -w .
	@echo "Code formatting completed!"