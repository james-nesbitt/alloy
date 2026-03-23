# Alloy Implementation Roadmap

This document outlines the development phases for the Alloy platform, transitioning from the core foundation to the modular plugin ecosystem.

## Phase 1: Core Foundation (Complete)
Establish the micro-kernel, secure communication, and basic orchestration.

- [x] **mTLS & PKI Infrastructure**: User-level Root CA and instance leaf certificates.
- [x] **IPC Transport**: Unified TCP/Unix socket support with URI-style addressing.
- [x] **XDG Compliance**: Standardized paths for config (`~/.config/alloy`) and runtime (`XDG_RUNTIME_DIR/alloy`).
- [x] **Message Bus**: Basic request/response and internal kernel message handling.
- [x] **Orchestration & Discovery**:
    - [x] **Instance Tracking**: Core writes `info.json` (PID, socket, metadata) to the runtime directory.
    - [x] **CLI Orchestration**: `alloy list` and `alloy stop` using the instance tracking.
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

## Phase 3: Core Plugins & Infrastructure Refinement (Complete)
Establishing the "Standard Library", durable plugin state, and kernel "thinning".

- [x] **Plugin State Persistence**:
    - [x] Kernel-managed `Save/Load` mechanism for plugin-local state.
    - [x] Host-provided KV storage interface for WASM guests.
- [x] **Core Plugins & Standard Services**:
    - [x] **Infrastructure Plugins**:
        - [x] **Local Storage (`storage`)**: Virtual Filesystem (WASI) provider.
        - [x] **Identity & Access (`iam`)**: Authorization and RBAC.
        - [x] **Command Manager (`command-manager`)**: Central registry for executable actions.
        - [x] **Event Bus (`events`)**: Advanced Pub/Sub and event filtering.
- [x] **Kernel "Thinning" & Decentralization**:
    - [x] **Decentralized Discovery**: Move `discover` logic from Kernel to `command-manager`.
    - [x] **Event-Driven Auditing**: Replace direct `AuditLogger` with `events` + a Logger plugin.
    - [x] **Middleware Interceptors**: Add high-performance "Pre-Route" hooks for `iam` authorization.
    - [x] **System Telemetry**: Distributed tracing (OTEL) and audit-linkage.
- [x] **Resource Lifecycle & Sandboxing**:
    - [x] Resource constraints (Global Memory Limits 500MB, Message Timeouts).
    - [x] Fine-grained WASI capability mapping (Plugin-isolated storage).

## Phase 4: Application Plugins & Frontends (In Progress)
Logic-heavy plugins and the user interface.

- [x] **Alloy TUI**: Terminal-based client (Bubbletea).
    - [x] **Modal Interface**: Normal, Insert, and Command modes.
    - [x] **Leader-Key Support**: `<space>` as a primary command entry point.
- [x] **Buffer Manager**: Shared state and concurrent editing via `buffer`.
- [x] **WIT-Bindgen Migration**:
    - [x] Migrate all 8 core plugins to the WASM Component Model (WIT).
    - [x] Standarize compilation with `just` and `tinygo`.
    - [x] Implement a robust Host/Guest SDK for WIT calls.
- [x] **Group Chat**: Real-time communication and presence.
- [x] **AI Agent**: LLM integration and tool-use orchestration.
- [ ] **Advanced Interface & Discoverability**:
    - [x] **Hierarchical Command Trees**: Support for nested key sequences (e.g., `<space> b l`) and namespaced plugin commands.
    - [x] **Command Metadata**: Extend `command-manager` to store shortcut keys and "Marginalia"-style annotations.
    - [ ] **Smart Command Bar (Consult/Vertico model)**: Fuzzy-filtering, multi-column results, and live previews.
    - [x] **Visual Mnemonic Feedback**: "Breadcrumb" display in the minibuffer during multi-key sequences.
- [x] **WASM SDK Standard Library**: Formally abstracted plugin development Kit.
- [x] **Security Hardening**: RBAC and fine-grained capability routing via `iam` interceptor.

## Phase 5: Team & Project Collaboration (In Progress)
Focusing Alloy on high-performance team workflows, shared context, and collaborative development.

- [x] **Data-Driven Dashboard Protocol**: 
    - [x] **WIT Dashboard Provider**: Define a standard `get-summary` and `get-actions` interface.
    - [x] **Summary Aggregator**: Create a registry for plugins to submit "status cards".
- [x] **Unified Project Context**:
    - [x] **Global Workspace Registry**: Sync project metadata across plugins (`ai`, `buffer`, `secrets`).
    - [x] **Cross-Plugin Context-Passing**: Automatic inclusion of project-relevant files in AI queries.
- [ ] **Real-time Team Coordination**:
    - [ ] **Multi-User Presence**: See active team members in the `buffer` and `chat` plugins.
    - [ ] **Conflict Resolution**: Operational Transformation (OT) or CRDT-lite for shared editor buffers.
    - [x] **Shared Secret Scoping**: Team-level vs. user-level secret partitioning in the `secrets` plugin.
- [x] **Arbitrated UI Layouts**:
    - [x] **Multi-Pane TUI Refactor**: Implement the "Split Frame" layout in the terminal frontend.
    - [x] **Project Layout Manifest**: Allow projects to define default pane arrangements (e.g., `layout.json`).
- [ ] **Collaborative Knowledge Graph**:
    - [ ] **Semantic Project Indexing**: Background indexing of documents and code for team-wide search.

## Phase 6: Advanced Refinement & Deployment
Bringing the system to full maturity and broader reach.

- [x] **Alloy GUI**: Wayland-compatible graphical client (**Stable Gio + Experimental Native**).
- [ ] **Alloy Web**: Browser-based access via Go-WASM bridge.
- [ ] **Modal Consolidation (The "Bridged" Approach)**: Unify Leader-menu (`SPC`) and Command Bar (`:`) into a single high-performance "Omni-palette" with auto-transition logic and scoped fuzzy searching.
- [x] **Hot-Reloading**: Allow WASM plugins to be updated or swapped without restarting the core. 
    - [x] Runtime manager reload support.
    - [ ] Automatic file watching.
- [ ] **Global Resource Limits Configuration**: CLI/Config based limits.
- [ ] **Telemetry Port Configuration**: Support dynamic or configurable telemetry ports to allow concurrent core instances in multi-tenant or testing environments.
