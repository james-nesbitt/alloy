# Alloy WIT Build Instructions

This document provides clear instructions for building the WIT-based Alloy components.

## 📋 Prerequisites

1. **Go 1.25+** - https://go.dev/dl/
2. **TinyGo 0.33+** - https://tinygo.org/getting-started/install/
3. **wit-bindgen** - `cargo install wit-bindgen-cli`
4. **Just** - `cargo install just` (or use provided scripts)

## 🛠️ Build Targets

### Core Targets

| Target | Description | Command |
|--------|-------------|---------|
| `generate-wit-bindings` | Generate WIT bindings | `just generate-wit-bindings` |
| `build-wasm` | Build all WASM plugins | `just build-wasm` |
| `build-core-wit` | Build WIT-based core backend | `just build-core-wit` |
| `build` | Build everything | `just build` |

### Plugin Targets

| Target | Description | Command |
|--------|-------------|---------|
| `build-plugin PLUGIN` | Build specific plugin | `just build-plugin health` |
| (all plugins) | Build all plugins | `just build-wasm` |

### GUI Targets

| Target | Description | Command |
|--------|-------------|---------|
| `build-gui-tui` | Build TUI interface | `just build-gui-tui` |
| `build-gui-web` | Build Web interface | `just build-gui-web` |
| `build-gui-all` | Build all GUIs | `just build-gui-all` |

### Test Targets

| Target | Description | Command |
|--------|-------------|---------|
| `test-wit` | Run WIT tests | `just test-wit` |
| `test` | Run all tests | `just test` |

### Utility Targets

| Target | Description | Command |
|--------|-------------|---------|
| `clean` | Clean build artifacts | `just clean` |
| `fmt` | Format code | `just fmt` |
| `help` | Show help | `just help` |

## 📝 Justfile Targets

```makefile
# Generate WIT bindings
generate-wit-bindings:
	@echo "Generating WIT bindings..."
	@wit-bindgen rust --out-dir pkg/wasm2/bindings/host/wit-rust wit/alloy.wit
	@wit-bindgen tiny-go --out-dir pkg/wasm2/bindings/guest wit/alloy.wit

# Build all WASM plugins
build-wasm: generate-wit-bindings
	@echo "Building WASM plugins..."
	@for plugin in ai buffer chat health iam project secrets tasks; do \
		just build-plugin "$$plugin"; \
	done

# Build specific plugin
build-plugin plugin_name:
	@echo "Building plugin: $$plugin_name"
	@mkdir -p build/wasm
	@plugin_dir="plugins/wasm/$$plugin_name"
	@if [ -f "$$plugin_dir/main_wit.go" ]; then \
		(cd "$$plugin_dir" && GOOS=wasip1 GOARCH=wasm tinygo build -target=wasi -o "../../../build/wasm/$$plugin_name.wasm" main_wit.go); \
	else \
		(cd "$$plugin_dir" && GOOS=wasip1 GOARCH=wasm tinygo build -target=wasi -o "../../../build/wasm/$$plugin_name.wasm" .); \
	fi

# Build WIT-based core backend
build-core-wit:
	@echo "Building WIT-based core backend..."
	@mkdir -p build
	@go build -o build/core-wit ./cmd/alloy-core-wit

# Build TUI interface
build-gui-tui:
	@echo "Building TUI interface..."
	@mkdir -p build
	@go build -o build/alloy-tui ./cmd/alloy-tui

# Build Web interface
build-gui-web:
	@echo "Building Web interface..."
	@mkdir -p build
	@go build -o build/alloy-web ./cmd/alloy-web

# Build all GUIs
build-gui-all: build-gui-tui build-gui-web

# Build everything
build: build-wasm build-core-wit build-gui-all

# Run tests
test-wit:
	@echo "Running WIT tests..."
	@cd pkg/wasm2/runtime && go test -v
	@cd pkg/wasm2 && go test -v
	@cd pkg/kernel && go test -v -run "TestWIT.*"

test: test-wit

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf build/

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@goimports -w .
```

## 🚀 Quick Start

```bash
# 1. Generate WIT bindings
just generate-wit-bindings

# 2. Build all WASM plugins
just build-wasm

# 3. Build core backend
just build-core-wit

# 4. Build GUIs
just build-gui-all

# OR build everything at once
just build

# Run tests
just test
```

## 🔧 Local Development Setup

For local development, set up a Go workspace:

```bash
# Initialize Go workspace
go work init
go work use .
go work use pkg/wasm2/bindings/guest
```

Or use replace directives in each plugin's go.mod:

```bash
# In each plugin directory:
go mod edit -replace github.com/jnesbitt/alloy-go/pkg/wasm2/bindings/guest=../../pkg/wasm2/bindings/guest
go mod tidy
```

## 📁 Output Structure

After building, the `build/` directory will contain:

```
build/
├── wasm/            # WASM plugins (.wasm files)
│   ├── ai.wasm
│   ├── buffer.wasm
│   ├── chat.wasm
│   ├── health.wasm
│   ├── iam.wasm
│   ├── project.wasm
│   ├── secrets.wasm
│   └── tasks.wasm
├── core-wit         # WIT-based core backend
├── alloy-tui        # TUI interface
└── alloy-web        # Web interface
```