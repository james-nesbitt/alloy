# Alloy Build and Plugin Guidelines

Alloy is a hybrid system that leverages both Native Go code for infrastructure and WebAssembly (WASM) for application logic. This document outlines how to build the core and its plugins.

## 1. Build Process

Alloy uses a `justfile` (via [just](https://github.com/casey/just)) to manage its build process.

### 1.1 Building the Core
The Alloy core is built as a standard Go binary:
```bash
just build-core
```

### 1.2 Building WASM Plugins
Alloy plugins are built into independent WebAssembly binaries using the `wasip1` target. These are decoupled from the core lifecycle and can be built separately:
```bash
just build-plugins
```
This command compiles all plugins found in `plugins/wasm/` and places the resulting `.wasm` files in `build/wasm/`.

### 1.3 Building Everything
To build both the core and all plugins at once:
```bash
just build-all
```

## 2. Plugin Loading and Discovery

The Alloy core does not have plugins "burned-in" or statically linked. Instead, it discovers them at runtime.

### 2.1 The `--wasm-plugins` Flag
When starting the core, you can specify one or more directories for the core to scan for WASM plugins:
```bash
./build/core --wasm-plugins ./build/wasm --wasm-plugins ./my-custom-plugins
```
The core will automatically:
1. Scan these directories for files ending in `.wasm`.
2. Derive a Plugin ID from the filename (e.g., `chat.wasm` -> `plugin-chat`).
3. Load the plugin asynchronously using the `RegistryManager`.

### 2.2 Manual Plugin Loading
You can also load specific WASM plugins using the `--wasm-plugin` flag:
```bash
./build/core --wasm-plugin ./build/wasm/chat.wasm
```

### 2.3 Provisioning Manifest
For complex setups involving multiple native and WASM plugins with specific constraints, use a provisioning manifest:
```bash
./build/core --provision ./tests/wasm_provision.json
```
The manifest is a JSON file defining a list of plugins to load.

### 2.4 Asynchronous Loading
Because WASM compilation (Ahead-of-Time compilation via `wazero`) can be resource-intensive, the core loads plugins in the background. A plugin is not immediately available for message routing; it becomes available once its code is compiled and it successfully executes its `_start` routine and registers with the host.

## 3. Testing Considerations

When writing tests for Alloy, you must account for the asynchronous nature of WASM plugin initialization.

### 3.1 Compilation Overhead
WASM plugins in Alloy are relatively large because they include the Go runtime. Compiling these modules into machine code during a test run can take significant time (up to 30 seconds or more per plugin depending on the environment).

### 3.2 The Polling Pattern
Tests should never use fixed `time.Sleep()` to wait for a plugin. Instead, use the **Discovery Polling Pattern**:
1. Connect to the core.
2. Periodically send a `discover` request to the `plugin-command-manager`.
3. Check the response to see if the desired plugin ID is listed in the available targets.
4. Continue with test logic only once the plugin is present.

Refer to `tests/application_test.go` for an example of this pattern.

### 3.3 Test Timeouts
Due to compilation overhead, tests involving WASM plugins should be run with a generous timeout. The default project timeout is set to 30s in the `justfile`, but heavy integration tests may require manual adjustment or polling loops with much longer deadlines.

## 4. Resource Constraints

All WASM plugins are executed within the `wazero` runtime with the following constraints:
- **Sandbox**: No direct access to host files, network, or environment unless explicitly mapped by the core.
- **Clock Support**: Plugins are provided with access to system wall time and monotonic time via WASI.
- **Panic Guards**: The core manages the lifecycle of plugin modules; if a plugin's `_start` goroutine panics, it is caught to prevent the host process from crashing, though the plugin may enter an error state.
