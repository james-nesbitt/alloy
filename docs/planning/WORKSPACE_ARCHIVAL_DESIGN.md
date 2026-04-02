# Architectural Design: Workspace Archival (.ark)

## Goal
To implement a portable, self-contained workspace format (`.ark`) that captures project state and history while maintaining architectural separation between the Kernel (infrastructure) and the Project Plugin (business logic).

## Principles
1. **Plugin-Led**: The `project` plugin is the owner of the archival domain.
2. **Kernel as Provider**: The kernel provides capabilities (Event Store, KV, FS) but does not understand "Project" semantics.
3. **Reproducibility**: An `.ark` archive must allow full reconstruction of the workspace state and historical replay.

## Implementation Status: Completed

The Workspace Archival system is fully implemented as of 2026-04-02.

### Phase 1: WIT Definition Update (Completed)
- `project:archive` and `project:restore` added to `wit/alloy.wit`.
- `state:export` and `state:import` patterns established.

### Phase 2: Project Plugin Orchestration (Completed)
- `archive` handler implemented.
- **Workflow**:
    - Broadcasts `quiesce` to all plugins.
    - Collects `state:export` responses.
    - Fetches event history from `history:get`.
    - Packages result into `.ark` (tar.gz) in `[data-dir]/project/`.

### Phase 3: Restoration & Migration Guard (Completed)
- `restore` handler implemented.
- **Workflow**:
    - Unpacks `.ark` bundle.
    - Restores metadata and dispatches `state:import` to plugins.
- Basic versioning present in manifest for future migration guards.

### Phase 4: Cold Storage Discovery (Completed)
- **Librarian Integration**: Archives are indexed by the Librarian upon creation.
- **Search**: `librarian:search` surfaces archived projects in Omni-Palette.
- **Management**: `project:list-archives` and `project:delete-archive` implemented for archive lifecycle management.
