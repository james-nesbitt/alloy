# Alloy Build System - WIT WASM Implementation

# Set shell
SHELL := "bash"

# Default target
default: help

# Show help
help:
	@echo "Alloy Build System - WIT WASM Implementation"
	@echo ""
	@echo "Available targets:"
	@echo "  generate-wit-bindings - Generate WIT bindings"
	@echo "  build-wasm          - Build all WASM plugins"
	@echo "  build-plugin PLUGIN - Build specific plugin"
	@echo "  build-core-wit      - Build WIT-based core backend"
	@echo "  test-wit            - Run WIT tests"
	@echo "  clean               - Clean build artifacts"
	@echo "  fmt                 - Format code"
	@echo "  build               - Build everything"
	@echo "  test                - Run all tests"
	@echo "  all                 - Build and test everything"

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
	@for plugin_dir in plugins/wasm/*; do \
		if [ -d "$$plugin_dir" ]; then \
			plugin_name=$(basename "$$plugin_dir"); \
			just build-plugin "$$plugin_name"; \
		fi; \
	done
	@echo "WASM plugins built successfully!"

# Build specific plugin
build-plugin plugin_name:
	@echo "Building plugin: $$plugin_name"
	@mkdir -p build/wasm
	@plugin_dir="plugins/wasm/$$plugin_name"
	@if [ -d "$$plugin_dir" ]; then \
		# Copy WIT bindings to the plugin directory
	mkdir -p "$$plugin_dir/wit"
	cp -r pkg/wasm2/bindings/guest/* "$$plugin_dir/wit/"
	
	# Build with TinyGo
	if [ -f "$$plugin_dir/main_wit.go" ]; then \
		(cd "$$plugin_dir" && GOOS=wasip1 GOARCH=wasm tinygo build -target=wasi -o "../../../build/wasm/$$plugin_name.wasm" main_wit.go); \
	else \
		(cd "$$plugin_dir" && GOOS=wasip1 GOARCH=wasm tinygo build -target=wasi -o "../../../build/wasm/$$plugin_name.wasm" .); \
	fi; \
	
	echo "Built: build/wasm/$$plugin_name.wasm"; \
	else \
		echo "Plugin directory not found: $$plugin_dir"; \
		exit 1; \
	fi

# Build the WIT-based core backend
build-core-wit:
	@echo "Building WIT-based core backend..."
	@mkdir -p build
	@go build -o build/core-wit ./cmd/alloy-core-wit
	@echo "Built: build/core-wit"

# Test WIT implementation
test-wit:
	@echo "Running WIT tests..."
	@cd pkg/wasm2/runtime && go test -v
	@cd pkg/wasm2 && go test -v
	@cd pkg/kernel && go test -v -run "TestWIT.*"
	@echo "WIT tests completed!"

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

# Run all tests
test: test-wit
	@echo "All tests completed!"

# Build everything
build: build-wasm build-core-wit
	@echo "Build completed!"

# Build and test everything
all: build test
	@echo "All tasks completed!"