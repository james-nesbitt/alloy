# PLANNER: Phase 7 Next Steps

Following the successful integration of the **Widget Manager** and the **Omni-Palette Bridge**, the infrastructure is ready for dynamic frontend orchestration. 

## 1. Dynamic TUI Workspace (`cmd/alloy-tui`)
The TUI currently uses a static layout. It must be refactored to support a plugin-driven viewport.

- **Refactor**: Replace the hardcoded `Pane` list in `main.go` with a dynamic collection managed by the `Update()` loop.
- **Subscriptions**: Subscribe the TUI client to `dashboard:widget-registered` and `dashboard:widget-updated`.
- **Logic**:
    - When a widget is registered, create a new Bubble-based viewport for it.
    - When updated, refresh the content of that specific viewport.
    - Implement a "Leader Mode" (Tab selection) to switch focus between these dynamic widgets.

## 2. Omni-Palette UI (TUI & Web)
While the bridge logic exists, the actual user-facing selection UI needs refinement.

- **TUI**: Implement an `omni-view` Bubble that captures keystrokes, sends `omni:search` to the Kernel, and displays a filtered list of results.
- **Web**: Enhance the results dropdown in the `omni-palette` template to handle the `response:omni:search` result format.
- **Intents**: Ensure that selecting a result correctly dispatches an intent-based message (e.g., `intent:open-buffer`, `intent:start-chat`).

## 3. GUI Dashboard Implementation (`cmd/alloy-gui-gio`)
With the build environment stabilized (Wayland/XKB dependencies met), we can now implement the native GUI dashboard.

- **Layout**: Implement a grid-based Gio layout.
- **Widget Rendering**: Create a generic `WidgetComponent` that renders Markdown or Text content provided by the Kernel's `WidgetManager`.
- **Live Updating**: Use the same event-driven "Push Model" to update the UI without polling.

## 4. Project Manifest Integration (`alloy-project.json`)
The Kernel should move away from hardcoded plugin loading.

- **Logic**: Implement `pkg/project/manifest.go` to parse an `alloy-project.json` file.
- **Auto-Bootstrapper**: On startup, the Kernel reads the manifest and automatically triggers `wasm.LoadPlugin()` for all plugins listed in the project's "tooling" or "dependencies".
- **Capability Negotiation**: Verify that all required capabilities for a project are satisfied by the loaded plugins.

## 5. Viewport Synchronization Test
Perform a "Grand Integration" test:
1. Start the Kernel, TUI, and Web frontends.
2. Load the `ai` plugin.
3. The `ai` plugin registers a "Status" widget.
4. **Verification**: Confirm the "AI Assistant" pane appears automatically in *both* the TUI and Web dashboards without a restart.
