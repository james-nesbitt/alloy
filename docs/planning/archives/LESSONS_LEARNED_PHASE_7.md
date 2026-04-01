# Phase 7 Lessons Learned: Omni-Palette & Widget Integration

This document captures key technical challenges, solutions, and architectural shifts discovered during the implementation of the Phase 7 milestones (Omni-Palette and Universal Widget System).

## 1. WIT & Wasm Toolchain Complexity

### 1.1 `wit-bindgen` Version Sensitivity
- **Observation**: Newer versions of `wit-bindgen` (e.g., v0.54.0+) have deprecated the `tiny-go` subcommand in favor of a universal `go` command.
- **Problem**: The new `go` command generates code using `runtime.Pinner`, which is **not supported by TinyGo**. This makes the generated code unusable for building WASM plugins.
- **Solution**: Revert to/use `wit-bindgen` **v0.17.0**. This version still includes the `tiny-go` generator which produces compatible code for the `wasm32-unknown-unknown` target.
- **Lesson**: Standardize the toolchain version across all development environments to avoid non-reproducible build failures.

### 1.2 TinyGo & `wasm-opt` Dependency
- **Observation**: TinyGo automatically attempts to use `wasm-opt` (from Binaryen) to optimize the output and handle `asyncify` transformation.
- **Problem**: In minimal environments (CI/CD or fresh dev setups), `wasm-opt` may be missing, causing the entire `tinygo build` to fail even if the compilation itself was successful.
- **Solution**: Implemented a mock `wasm-opt` script in `build/tmp/bin/wasm-opt` that parses the `--output <path>` flag and simply copies the input to the output.
- **Lesson**: Provide a clear "mocking" or "skip-optimization" path in the `justfile` for environments where the full Binaryen suite isn't strictly required for sanity checks.

## 2. Architectural Shift: Kernel-Managed State

### 2.1 The Rise of the `WidgetManager`
- **Previous State**: Plugins managed their own "views" or "widgets" locally, and the WASM runtime tried to track them.
- **New State**: Shifted widget registration and content tracking to a centralized `WidgetManager` core service in `pkg/kernel/registry.go`.
- **Benfit**: This allows for a **"Single Source of Truth"** for the dashboard. When a plugin calls `update_widget`, the Kernel can broadcast a `dashboard:widget-updated` event to **all** frontends simultaneously (Web, TUI, and GUI).
- **Implementation Note**: Host-calls from the WASM runtime (`internalUpdateWidget`) now create standard Alloy messages and route them through the kernel instead of modifying local runtime state.

## 3. Frontend Real-time Synchronization

### 3.1 Debounced Search & Bridge Logic
- **Discovery**: Real-time search across multiple WASM plugins (Omni-Palette → Indexer/Buffers) can be chatty over WebSockets.
- **Solution**: Added client-side debouncing in `bridge.js` for the `omni:search` input.
- **Bridge Refinement**: The `bridge.js` now handles `metadata.action` fields in search results (e.g., `buffer:open`), allowing the frontend to dispatch specific intents back to the kernel upon result selection.

## 4. Binding Generation and `go.mod`

### 4.1 Stale Bindings & Import Paths
- **Observation**: Changing the `wit/alloy.wit` file requires re-running `wit-bindgen`. However, if the generated `alloy` package path in the host code changes, Go's modules can become confused.
- **Solution**: Use `go work sync` and ensure the `just generate` recipe explicitly cleans/re-initializes the `go.mod` for the generated guest bindings (`build/gen/bindings/guest/go.mod`).
- **Lesson**: Treat generated bindings as a first-class internal module and use `replace` directives in `go.work` or plugin `go.mod` files to point to the `build/gen` directory.
