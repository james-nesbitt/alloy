# Alloy Build & Plugin Guide

This document provides a concise reference for building and managing the Alloy **Modular Workspace Engine**.

---

## 🚀 Prerequisites
- **Go 1.25+**
- **TinyGo 0.35+**
- **wit-bindgen-cli 0.36.0**
- **Just** (Command runner)

## 🛠 Build Commands (`just`)

| Command | Action |
|---------|--------|
| `just build-all` | Full build (Core + Plugins + Frontends) |
| `just build-core` | Build the Go backend kernel |
| `just build-plugins` | Build all WASM plugins |
| `just generate` | Refresh WIT bindings |
| `just test` | Run full integration suite |
| `just setup-dev` | Configure local `go.work` and module replacements |

## 📁 Build Output (`./build/dist/`)
Alloy follows a standard FHS-like layout for builds:
- `bin/alloy`: Unified CLI entry point.
- `bin/alloy-tui`: Terminal UI client.
- `libexec/alloy/alloy-core`: Main backend kernel.
- `lib/alloy/plugins/*.wasm`: Compiled application logic.

## 🔌 Plugin Architecture
Alloy is a **Modular Workspace Engine**:
- **Coordination Kernel (Go)**: High-performance infrastructure (IAM, KV, Messages, Events, Capabilities).
- **Runtime Plugins (WASM)**: Isolated application logic for **Team Cooperation** (AI, Chat, Buffer, Projects).

### Strategy & Discovery
1. **Directory-based**: The kernel scans `--wasm-plugins` or uses a `provision.json` manifest.
2. **Async Loading**: Plugins are compiled by `wazero` in the background. Use the **Discovery Polling Pattern** in tests to wait for a plugin to reach `Running` status.
3. **Isolation**: Each plugin is sandboxed with specific memory and capability (WASI) limits.

## 🛠 Troubleshooting WASM
If `wasm-opt` is missing during `just build-plugins`:
- The build script attempts to download `binaryen` into `./build/tmp/bin`.
- Ensure `tinygo` is correctly in your `$PATH`.
