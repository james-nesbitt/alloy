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
Hardening the core for multi-user, multi-modal interaction.
- [x] **Omni-Palette**: Unified search and command entry.
- [ ] **Universal Modal Engine**: Common state machine for Vim/Helix/Meow navigation.
- [ ] **Resource Isolation**: Strict CPU/Memory limits to protect shared workspaces.
- [x] **Hot-Reloading**: Seamless plugin updates without restart.
- [ ] **Alloy Web**: Browser-based access via Go-WASM bridge.

## Phase 7: Project Orchestration (The Shared Effort)
Defining the "What": The contract for a team’s coordination.
- [ ] **Project Manifest Schema**: `alloy-project.json` for plugin and capability requirements.
- [ ] **Archetype Bootstrapping**: Automated loading of project-specific plugin suites (Coding, Sales, Support, Ops).
- [ ] **Capability Negotiation & Fallbacks**: Matching project requirements to the best available providers (e.g., Semantic vs. Basic search).
- [ ] **Heuristic Discovery**: Zero-config project detection based on existing local files (e.g., `go.mod`, `.git`).

## Phase 8: Contextual Identity & Roles (The "Who")
Defining the "Participants": Mapping verified actors to project-specific permissions.
- [ ] **Contextual Role Mapping**: Linking mTLS/Peer identities to project-specific roles (Editor, Planner, Reviewer).
- [ ] **Role-Filtered Messaging**: IAM-level filtering of events and RPC calls based on the active role.
- [ ] **Capability Visibility**: Automatically masking UI elements and commands based on role permissions.

## Phase 9: Workspace Synthesis (The "Lens")
Defining the "Personal View": Merging project tools with user-specific preferences.
- [ ] **Workspace Composition Engine**: Frontend logic to merge the **Project Substrate** with the **User Profile**.
- [ ] **User Plugin Side-cars**: Personal tools (Private Notes, Time Trackers) running in the user’s local context.
- [ ] **Multi-Project Switcher**: UI layer for bridging multiple active efforts (projects) in a single frontend session.

## Phase 10: Advanced Team Dynamics (The "Cooperation")
Refining real-time interaction and automated coordination.
- [ ] **Intent-Based Routing**: Shifting from "Call Plugin X" to "Fulfill Intent Y" (e.g., `intent:request-review`).
- [ ] **Headless Actor Framework**: First-class support for AI agents and automation scripts as project participants.
- [ ] **Presence & Activity Sync**: Real-time visualization of team focus and idle states across archetypes.
- [ ] **Omni-Channel Bridging**: Standardized adapters for external tools (GitHub, Slack, Discord).

## Phase 11: Lifecycle & Audit (The "History")
Ensuring the sustainability and traceability of the shared effort.
- [ ] **The Librarian Service**: Cross-plugin semantic indexing of project decisions and change history.
- [ ] **Temporal Playback (Time Travel)**: Viewports to rewind and audit a project's event stream.
- [ ] **Workspace Archival (.ark)**: One-click extraction of project state, buffers, and metadata into a portable format.
- [ ] **Resource Token Delegation**: Secure, time-limited lending of personal credentials (API keys) for project tasks.
