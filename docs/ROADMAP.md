# Alloy Implementation Roadmap

This document outlines the development phases for the Alloy platform. 

---

## Phase 1: Core Foundation (Complete)
- [x] **mTLS & PKI Infrastructure**
- [x] **IPC Transport** (Unified TCP/Unix socket support)
- [x] **XDG Compliance**
- [x] **Message Bus**
- [x] **Service Discovery**
- [x] **Audit & Logging**

## Phase 2: WASM Enablement (Complete)
- [x] **WASM Runtime (`pkg/wasm`)** using `wazero`
- [x] **Host Discovery Interface**
- [x] **Standard Guest ABI**
- [x] **Go-based Guest SDK**

## Phase 3: Core Plugins (Complete)
Establishing the "Standard Library" services.
- [x] **Storage Service**: Virtual Filesystem (WASI) provider.
- [x] **IAM Service**: Identity and RBAC.
- [x] **Command Manager**: Central action registry.
- [x] **Events Service**: Advanced Pub/Sub.
- [x] **Logger Plugin**: Event-driven auditing.

## Phase 4: Application Plugins & UI (Complete)
Expanding the feature set and user interaction.
- [x] **Alloy TUI**: Bubbletea terminal client.
- [x] **Buffer Manager**: Shared concurrent editing.
- [x] **WIT Migration**: Full adoption of the WASM Component Model.
- [x] **Group Chat**: Real-time communication and presence.
- [x] **AI Agent**: LLM tool-use orchestration.
- [x] **Command Trees**: Hierarchical key sequences.

## Phase 5: Team & Project Collaboration (Complete)
- [x] **Data-Driven Dashboard**: Summaries and action cards.
- [x] **Unified Project Context**: Workspace registry and shared secrets.
- [x] **Multi-User Presence**: Visual indicators for team activity.
- [x] **Conflict Resolution**: OT/CRDT-lite for editor buffers.
- [x] **Knowledge Graph**: Semantic indexing and team-wide search.

## Phase 6: Advanced Refinement (In Progress)
- [x] **Alloy GUI**: Stable Wayland-compatible client (Gio).
- [ ] **Alloy Web**: Browser-based access via Go-WASM bridge.
- [x] **Omni-Palette**: Unified search and command entry.
- [ ] **Universal Modal Interface**: Unified navigation state across TUI, GUI, and Web.
- [ ] **Configurable Modal Engine**: Choice of Neovim, Helix, or Emacs-Meow interaction sets.
- [x] **Hot-Reloading**: Seamless plugin updates without restart.
- [ ] **Resource Isolation**: Configurable CPU/Memory limits.
