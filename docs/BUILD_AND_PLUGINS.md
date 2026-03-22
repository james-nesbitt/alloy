# Alloy Build and Plugin Guidelines

Alloy is a hybrid system that leverages both Native Go code for infrastructure and WebAssembly (WASM) for application logic. This document outlines how to build the core and its plugins.

## 1. Prerequisites

Before building Alloy, ensure you have the following tools installed:

1. **Go 1.25+** - [go.dev](https://go.dev/dl/)
2. **TinyGo 0.33+** - [tinygo.org](https://tinygo.org/getting-started/install/)
3. **wit-bindgen** - `cargo install wit-bindgen-cli`
4. **Just** - `cargo install just` (optional, but highly recommended)

## 2. Build Process

Alloy uses a `justfile` (via [just](https://github.com/casey/just)) to manage its build process.

### 2.1 Standard Build Targets

| Target | Description | Command |
|--------|-------------|---------|
| `all` | Build everything (Core, Plugins, GUIs, CLI) | `just all` |
| `build-core` | Build the Alloy Core (Backend) | `just build-core` |
| `build-plugins`| Build all WASM plugins | `just build-plugins` |
| `build-binaries`| Build all host binaries (Core, TUI, GUI, CLI) | `just build-binaries` |
| `build-plugin NAME` | Build a single plugin (e.g., `health`) | `just build-plugin health` |
| `generate` | Regenerate WIT bindings | `just generate` |
| `setup-dev` | Configure Go work and module replacements | `just setup-dev` |

### 2.2 Output Structure

After building, the `./build/` directory will contain:

```text
build/
├── bin/             # Host binaries
│   ├── alloy-core   # Main backend
│   ├── alloy-tui    # Terminal interface
│   ├── alloy    # Command line tool
│   └── alloy-gui    # Gio-based native GUI
└── wasm/            # Compiled WASM plugins
    ├── ai.wasm
    ├── buffer.wasm
    ├── chat.wasm
    ├── health.wasm
    ├── iam.wasm
    ├── project.wasm
    ├── secrets.wasm
    └── tasks.wasm
```

## 3. Plugin Strategy

Alloy plugins are built into independent WebAssembly binaries using the `wasip1` target. These are decoupled from the core lifecycle and can be built separately.

### 3.1 The Plugin Manager
The Alloy core discovers plugins at runtime. When starting the core, it scans specified directories for `.wasm` files.

### 3.2 Loading Plugins
You can specify plugin directories via command-line flags when running `alloy-core`:
```bash
./build/bin/alloy-core --data-dir ./data --provision provision.json
```

### 3.3 Asynchronous Loading
WASM compilation (utilizing the `wazero` runtime) is performed in the background. A plugin becomes available once its code is compiled and it successfully executes its initialization routines via the WIT `alloy-init` function.

## 4. Development & Testing

### 4.1 Development Setup
If you are developing new plugins or modifying the SDK, run:
```bash
just setup-dev
```
This script automatically regenerates the `go.work` file and configures the necessary `replace` directives in the plugin `go.mod` files to point to your local code instead of GitHub URI references.

### 4.2 Running Tests
To run the full test suite, including WIT integration tests:
```bash
just test
```
To run only WASM-specific implementation tests:
```bash
just test-wasm
```

### 4.3 Testing Constraints
Due to the compilation overhead of large WASM modules, tests involving plugins should account for significant startup latency. Use the **Discovery Polling Pattern** instead of fixed sleeps:
1. Connect to the core.
2. Periodically check plugin availability via the kernel's metadata registry.
3. Proceed once the plugin status is marked as `Running`.

## 5. Troubleshooting WASM Builds

If TinyGo fails to build because of a missing `wasm-opt`:
1. Alloy provides a mechanism to download `binaryen` into `./build/tmp/bin` automatically if needed.
2. The `scripts/build-plugin.sh` looks for `./build/tmp/bin/wasm-opt` and uses it for final optimization passes.
3. If you still encounter issues, ensure `WASMOPT` is correctly set in your environment or follow the instructions in `scripts/build-plugin.sh`.
