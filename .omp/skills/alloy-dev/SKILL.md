---
name: alloy-dev
description: Provides specialized workflows, setup instructions, and helper scripts for working on Alloy.
---

# Alloy Dev Skill

This skill provides centralized instructions for editing WIT, building WASM plugins, and running tests in the Alloy project.

## Workflow

### 1. WIT Evolution
When a plugin needs a new capability, first update `wit/alloy.wit`.
Then run:
```bash
just build-all
```
This regenerates the bindings in `build/gen/bindings/`.

### 2. Plugin Implementation
WASM plugins are in `plugins/wasm/`.
- `main_wit.go`: Standard entry point using the Alloy Guest SDK.
- Use `pkg/wasm/guest/sdk.go` as the primary interaction surface for guest code.

### 3. Verification Protocol
Before any merge, the following must pass:
1.  `just fmt` (Standardizes Go style)
2.  `just build-all` (Builds all plugins and frontends)
3.  `just test-all` (Runs all kernel, plugin, and frontend tests)

## Directories
- `api/`: Shared messaging models (`Message`, `Capability`, `Workspace`).
- `pkg/kernel/`: Native Go core services (IAM, Buffer, EventBus).
- `pkg/wasm/runtime/`: The `wazero`-based plugin execution host.
- `cmd/alloy-tui/`: Bubble Tea-based terminal UI client.
- `cmd/alloy-web/`: Browser-based dashboard gateway.
- `plugins/wasm/`: Application logic in isolated sandboxes.
