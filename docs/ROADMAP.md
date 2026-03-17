# Alloy Implementation Roadmap

This document outlines the development phases for the Alloy platform, transitioning from the core foundation to the modular plugin ecosystem.

## Phase 1: Core Foundation (Current)
Establish the micro-kernel, secure communication, and basic orchestration.

- [x] **mTLS & PKI Infrastructure**: User-level Root CA and instance leaf certificates.
- [x] **IPC Transport**: Unified TCP/Unix socket support with URI-style addressing.
- [x] **XDG Compliance**: Standardized paths for config (`~/.config/alloy`) and runtime (`XDG_RUNTIME_DIR/alloy`).
- [x] **Message Bus**: Basic request/response and internal kernel message handling.
- [x] **Orchestration & Discovery**:
    - [x] **Instance Tracking**: Core writes `info.json` (PID, socket, metadata) to the runtime directory.
    - [x] **CLI Orchestration**: `alloy-cli list` and `alloy-cli stop` using the instance tracking.
    - [x] **Service Discovery**: Kernel method for clients to query active targets and their capabilities.
- [x] **Audit & Logging**:
    - [x] **Structured Audit Log**: Persistent, tamper-evident record of identity-verified events.

## Phase 2: WASM Enablement
Transform the kernel into a true plugin host.

- [ ] **WASM Runtime (`pkg/wasm`)**:
    - [ ] Integrate `wazero` for high-performance, sandboxed execution.
    - [ ] Implement the **Host Discovery Interface**: How WASM plugins "see" the outside world.
    - [ ] Standardize the **Guest ABI**: How plugins receive and send `api.Message` payloads.
- [ ] **Plugin Lifecycle**:
    - [ ] Kernel logic to load, initialize, and monitor `.wasm` modules.
    - [ ] Resource constraints (CPU Fuel, Memory Limits).
- [ ] **Initial Guest SDK**: Go/Rust libraries for building Alloy-compatible plugins.

## Phase 3: Core Plugins (The "Standard Library")
Implementing functional blocks as isolated plugins (see [Plugin Details](PLUGINS_ROADMAP.md)).

1. **Local Storage (`plugin-storage`)**: Virtual Filesystem (WASI) and Key-Value store.
2. **Identity & Access (`plugin-iam`)**: Moving from "Verified Identity" to "Authorized Actions" (RBAC).
3. **Command Manager (`plugin-command-manager`)**: Centralized registry for executable actions.
4. **Registry Manager (`plugin-registry-manager`)**: Handling plugin downloads and hot-updates.

## Phase 4: Application Plugins
Logic-heavy plugins that provide the user experience.

- **Buffer Manager**: Shared state and concurrent editing.
- **Group Chat**: Real-time communication and presence.
- **AI Agent**: LLM integration and tool-use orchestration.

## Phase 5: Frontends
Bringing the system to users.

- **Alloy TUI**: Terminal-based client (Bubbletea).
- **Alloy GUI**: Wayland-compatible graphical client.
- **Alloy Web**: Browser-based access via Go-WASM bridge.
