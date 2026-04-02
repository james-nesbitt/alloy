# Architectural Design: Workspace Archival (.ark)

## Goal
To implement a portable, self-contained workspace format (`.ark`) that captures project state and history while maintaining architectural separation between the Kernel (infrastructure) and the Project Plugin (business logic).

## Principles
1. **Plugin-Led**: The `project` plugin is the owner of the archival domain.
2. **Kernel as Provider**: The kernel provides capabilities (Event Store, KV, FS) but does not understand "Project" semantics.
3. **Reproducibility**: An `.ark` archive must allow full reconstruction of the workspace state and historical replay.

## Implementation Plan

### Phase 1: WIT Definition Update
- Add `project:archive` and `project:restore` to the `alloy:project` interface in `wit/alloy.wit`.
- Define `state:export` and `state:import` as common capability patterns for all stateful plugins.

### Phase 2: Project Plugin Orchestration
- Implement `archive` handler in `plugins/wasm/project/main_wit.go`.
- **Workflow**:
    - Broadcast `quiesce` to all plugins.
    - Collect `state:export` responses.
    - Fetch event history from `history:get`.
    - Package into a `.tar.gz` bundle (the `.ark` file) using the Host's filesystem capability.

### Phase 3: Restoration & Migration Guard
- Implement `restore` handler in `plugins/wasm/project/main_wit.go`.
- **Workflow**:
    - Unpack `.ark` bundle.
    - Match `state/` files to registered plugins.
    - Dispatch `state:import` to respective plugins.
    - Re-initialize the Kernel's History store with the archived event trace.
- Implement a basic **Migration Guard** that handles version checks between the archive manifest and the current Alloy runtime.

### Phase 4: Cold Storage Discovery
- Update the **Librarian** to surface archived projects in Omni-Palette search results using metadata headers stored in a global project registry.
