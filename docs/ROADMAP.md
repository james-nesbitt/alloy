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

## Phase 3: Core Plugins & Infrastructure Refinement
Establishing the "Standard Library", durable plugin state, and kernel "thinning".

- [x] **Plugin State Persistence**:
    - [x] Kernel-managed `Save/Load` mechanism for plugin-local state.
    - [x] Host-provided KV storage interface for WASM guests.
- [x] **Core Plugins & Standard Services**:
    - [x] **Infrastructure Plugins**:
        - [x] **Local Storage (`plugin-storage`)**: Virtual Filesystem (WASI) provider.
        - [x] **Identity & Access (`plugin-iam`)**: Authorization and RBAC.
        - [x] **Command Manager (`plugin-command-manager`)**: Central registry for executable actions.
        - [x] **Event Bus (`plugin-events`)**: Advanced Pub/Sub and event filtering.
    - [x] **Standard Service Helpers**:
        - [x] **KV Store (`plugin-kv`)**: Simple persistent Key-Value service.
        - [x] **Cache Manager (`plugin-cache`)**: High-speed transient storage.
        - [x] **Doc Store (`plugin-doc`)**: Indexed document/search service.
        - [x] **Secret Manager (`plugin-secrets`)**: Policy-based encrypted storage.
    - [x] **Operations Core**:
        - [x] **Health & Monitoring (`plugin-health`)**: Resource tracking and heartbeat.
        - [x] **Task Runner (`plugin-tasks`)**: Scheduled and background job management.
        - [x] **Network Manager (`plugin-network`)**: Policy-enforced network/Fetch provider.
- [x] **Kernel "Thinning" & Decentralization**:
    - [x] **Decentralized Discovery**: Move `discover` logic from Kernel to `plugin-command-manager`.
    - [x] **Event-Driven Auditing**: Replace direct `AuditLogger` with `plugin-events` + a Logger plugin.
    - [x] **Middleware Interceptors**: Add high-performance "Pre-Route" hooks for `plugin-iam` authorization.
    - [x] **System Telemetry**: Distributed tracing (OTEL) and audit-linkage.
- [x] **Resource Lifecycle & Sandboxing**:
    - [x] Resource constraints (Global Memory Limits 500MB, Message Timeouts).
    - [x] Fine-grained WASI capability mapping (Plugin-isolated storage).

## Phase 4: Application Plugins & Frontends
Logic-heavy plugins and the user interface.

- [x] **Alloy TUI**: Terminal-based client (Bubbletea).
    - [x] **Modal Interface**: Normal, Insert, and Command modes.
    - [x] **Leader-Key Support**: `<space>` as a primary command entry point.
- [x] **Buffer Manager**: Shared state and concurrent editing via `plugin-buffer-manager`.
- [ ] **Advanced Interface & Discoverability**:
    - [ ] **Hierarchical Command Trees**: Support for nested key sequences (e.g., `<space> b l`) and namespaced plugin commands.
    - [ ] **Command Metadata**: Extend `plugin-command-manager` to store shortcut keys and "Marginalia"-style annotations.
    - [ ] **Smart Command Bar (Consult/Vertico model)**: Fuzzy-filtering, multi-column results, and live previews.
    - [ ] **Visual Mnemonic Feedback**: "Breadcrumb" display in the minibuffer during multi-key sequences.
- [ ] **WASM SDK Standard Library**: Formally abstracted plugin development Kit.
- [ ] **Security Hardening**: RBAC and fine-grained capability routing.
- [ ] **Group Chat**: Real-time communication and presence.
- [ ] **AI Agent**: LLM integration and tool-use orchestration.

## Phase 5: Advanced Refinement & Deployment
Bringing the system to full maturity.

- [x] **Alloy GUI**: Wayland-compatible graphical client (**Stable Gio + Experimental Native**).
- [ ] **Alloy Web**: Browser-based access via Go-WASM bridge.
- [x] **Hot-Reloading**: Allow WASM plugins to be updated or swapped without restarting the core. 
    - [x] Runtime manager reload support.
    - [ ] Automatic file watching.
- [ ] **Global Resource Limits Configuration**: CLI/Config based limits.
