# Phase 9 Planning: Workspace Synthesis

Phase 9 focuses on the **Frontend Composition Engine**, which integrates project-mandated tools from the project manifest with user-specific "Side-car" plugins. This phase will realize the goal of a personalized but project-aligned developer experience.

## Goals

1.  **Composition Engine**: A mechanism in the `frontend` or `widget-manager` to merge two sources of tools:
    *   **Project Tools**: Defined in `alloy-project.json` (e.g., specific project-only widgets, shared buffers).
    *   **User Side-cars**: Defined in a user's global configuration (e.g., personal AI agents, global search, private secrets).
2.  **User Side-car Support**: Implement a way for the `wasm-manager` to automatically load a set of user-defined plugins on startup, regardless of the active project.
3.  **Multi-Project Switcher**: A widget (likely in the Omni-Palette or a dedicated sidebar) that allows users to switch between their authorized project contexts.
4.  **Context-Aware Layouts**: Ensuring the `widget-manager` and `omni-palette` respond dynamically to the active context, updating the visible tools and commands.

## Architecture

### 1. Composition Engine (`pkg/frontend/compose.go`)
The `Compose` function will take a `UserConfig` and a `ProjectManifest` and return a unified `WorkspaceState`.
*   **Merge Strategy**: User tools are typically "always-on" (side-cars), while project tools are "contextual".
*   **Conflict Resolution**: If a user tool and project tool share the same ID, the project tool should usually take precedence unless it's a "global-override" user tool.

### 2. User Side-cars (`~/.alloy/user-config.json`)
The `wasm-manager` will be updated to:
1.  Look for a global `user-config.json`.
2.  Load specified plugins as `Global` scope.
3.  Ensure these plugins are available across all project contexts.

### 3. Project Switcher (`plugins/wasm/switcher`)
A new WASM plugin to:
1.  Query the `iam` service for all namespaces/contexts the user has access to.
2.  Provide a UI (via `widget-manager`) or command (via `omni-palette`) to switch the active `context-id`.
3.  Notify the Frontend to re-compose the workspace.

## Implementation Steps

### Task 1: Composition Logic & Frontend Updates
*   [x] Define `UserConfig` struct and persistence (likely in `pkg/storage`).
*   [x] Implement `ComposeWorkspace` utility.
*   [x] Update `pkg/frontend` types to handle a "composed" list of plugins and widgets.

### Task 2: Global Side-car Loading
*   [x] Update `wasm-manager` to load plugins from `~/.alloy/plugins/`.
*   [x] Implement "Global" capability registration (Namespace `*` or `global`).

### Task 3: The Project Switcher Plugin
*   [x] Create `plugins/wasm/switcher` plugin.
*   [x] Implement `switcher:list-projects` and `switcher:switch-to`.
*   [x] Update `omni-palette` to surface the switcher.

### Task 4: Dynamic Re-Composition
*   [x] Implement an event `system:context-changed`.
*   [x] Frontends (TUI, Web, GUI) listen for this event and trigger a full re-render/re-composition.

## Success Criteria
- [ ] Users can see both project-specific widgets and their personal "side-car" widgets simultaneously.
- [ ] Switching projects via the `switcher` plugin immediately updates the Omni-Palette commands and visible widgets.
- [ ] User-side-cars are loaded automatically on start and persist across project switches.
- [ ] Security boundaries (from Phase 8) are strictly enforced during and after context switches.

---

**Next Step**: Implementation of the `UserConfig` and `Side-car` loading in `wasm-manager`.
