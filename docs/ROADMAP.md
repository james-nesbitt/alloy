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

## Phase 2: WASM Enablement (Complete)
Transform the kernel into a true plugin host.

- [x] **WASM Runtime (`pkg/wasm`)**:
    - [x] Integrate `wazero` for high-performance, sandboxed execution.
    - [x] Implement the **Host Discovery Interface**: How WASM plugins "see" the outside world.
    - [x] Standardize the **Guest ABI**: How plugins receive and send `api.Message` payloads.
- [x] **Plugin Lifecycle**:
    - [x] Kernel logic to load, initialize, and monitor `.wasm` modules.
- [x] **Initial Guest SDK**: Go libraries for building Alloy-compatible plugins.

## Phase 3: Core Plugins & Lifecycle Persistence
Establishing the "Standard Library" and durable plugin state.

- [ ] **Plugin State Persistence**:
    - [ ] Kernel-managed `Save/Load` mechanism for plugin-local state.
    - [ ] Host-provided KV storage interface for WASM guests.
- [ ] **Core Plugins & Standard Services**:
    - [ ] **Infrastructure Plugins**:
        - [ ] **Local Storage (`plugin-storage`)**: Virtual Filesystem (WASI) provider.
        - [ ] **Identity & Access (`plugin-iam`)**: Authorization and RBAC.
        - [ ] **Command Manager (`plugin-command-manager`)**: Central registry for executable actions.
        - [ ] **Event Bus (`plugin-events`)**: Advanced Pub/Sub and event filtering.
    - [ ] **Standard Service Helpers**:
        - [ ] **KV Store (`plugin-kv`)**: Simple persistent Key-Value service.
        - [ ] **Cache Manager (`plugin-cache`)**: High-speed transient storage.
        - [ ] **Doc Store (`plugin-doc`)**: Indexed document/search service.
        - [ ] **Secret Manager (`plugin-secrets`)**: Policy-based encrypted storage.
    - [ ] **Operations Core**:
        - [ ] **Health & Monitoring (`plugin-health`)**: Resource tracking and heartbeat.
        - [ ] **Task Runner (`plugin-tasks`)**: Scheduled and background job management.
        - [ ] **Network Manager (`plugin-network`)**: Policy-enforced network/Fetch provider.
- [ ] **Resource Lifecycle**:
    - [ ] Resource constraints (CPU Fuel, Memory Limits).

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
