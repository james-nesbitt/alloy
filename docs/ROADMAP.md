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

## Phase 6: The Collaborative Foundation (In Progress)
Refining the core for multi-user, multi-modal interaction.
- [x] **Omni-Palette**: Unified search and command entry.
- [ ] **Universal Modal Engine**: Common state machine for Vim/Helix/Meow navigation.
- [ ] **Resource Isolation**: Strict CPU/Memory limits for plugin stability.
- [x] **Hot-Reloading**: Seamless plugin updates without restart.
- [ ] **Alloy Web**: Browser-based access via Go-WASM bridge.

## Phase 7: Project Orchestration (The "What")
Defining project scopes through manifests and archetypes.
- [ ] **Project Manifest Schema**: `alloy-project.json` for plugin and capability requirements.
- [ ] **Archetype Bootstrapping**: Automated loading of plugin suites (Coding, Sales, Support, Ops).
- [ ] **Capability-Based Discovery**: Decoupling functionality from specific plugin IDs using WIT interfaces.

## Phase 8: Contextual Identity & Roles (The "Who")
Mapping users to project roles and enforcing capability-based access.
- [ ] **Project-Specific Role Mappings**: Contextual RBAC linking mTLS identities to roles (Editor, Planner, Reviewer).
- [ ] **Role-Filtered Messaging**: IAM-level filtering of requests and events based on active role.
- [ ] **Capability Visibility**: Automatically hiding/disabling commands in the Omni-palette that exceed a user's role permissions.

## Phase 9: Workspace Composition (The "Lens")
Merging project requirements with user-specific preferences and private tools.
- [ ] **Frontend Composition Engine**: Three-way merge of Project Substrate, Role Capabilities, and User Profile.
- [ ] **User Plugin Side-cars**: Personal plugins running in the user context, isolated from the team core.
- [ ] **Profile-Driven Interface**: User-selectable themes, keybinding drivers, and private dashboard widgets.

## Phase 10: Advanced Team UX (The "Cooperation")
Refining the real-time collaborative experience.
- [ ] **Collaborative Workspace Editing**: Tooling for admins to live-edit the shared project dashboard.
- [ ] **Presence & Activity Sync**: Real-time visualization of team focus and idle states across archetypes.
- [ ] **Shared State persistence**: CRDT-lite synchronization for structured data (tasks, leads, schedules).
