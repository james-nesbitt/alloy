# PHASE 7: OMNI-PALETTE & UNIVERSAL VIEWPORT

## 1. Universal Omni-Palette Integration
The goal is to make the `Ctrl+P` (or `F1`) experience consistent across TUI, GUI, and Web by leveraging the `omni-palette` WASM plugin correctly.

### 1.1 Web Frontend Integration
- **Debounced WebSocket Search**: Modify `bridge.js` to send `omni:search` requests to the kernel as the user types.
- **Async Result Rendering**: Update the `omni-results` DOM element whenever a `response` message for search arrives.
- **Intent Dispatch**: Ensure selecting a result triggers the correct plugin action (e.g., `buffer:open`, `chat:send`).

### 1.2 TUI Integration
- **Omni-Palette View**: Create a dedicated Bubbles-based list component for the palette.
- **Active Search**: Route keypresses in the palette view to `kernel.RouteMessage` targeting the `omni-palette` plugin.

---

## 2. Dynamic Viewport & Widget System
Transform the static layout into a dynamic, plugin-driven dashboard.

### 2.1 Widget Registry
- Implement a host-side registry in `pkg/kernel/registry.go` for "Widgets".
- A widget consists of: `ID`, `PluginID`, `Title`, `ContentType` (Markdown/HTML/Text), and `Content`.

### 2.2 Viewport Push Model
- When a plugin updates its widget content via `plugin.UpdateWidget()`, the kernel should broadcast this update to all connected frontends.
- **Web**: React/Vue (or bridge.js) updates a specific `<div>` by ID.
- **TUI**: Re-render the specific pane associated with that widget ID.

---

## 3. Implementation Steps

### Step 1: Web Search Routing
- [ ] Add debouncing to `bridge.js` search input.
- [ ] Implement `omni:search` message handler in `cmd/alloy-web/handlers.go` (if not already routing correctly).
- [ ] Update `bridge.js` to handle `response` type messages specifically for search.

### Step 2: Widget Protocol
- [ ] Define the `WidgetUpdate` message structure.
- [ ] Update `pkg/wasm/runtime/runtime.go` to provide a host-call for `update_widget`.
- [ ] Implement the registry in the kernel to track which widgets are registered.

### Step 3: TUI Workspace Layout
- [ ] Refactor `cmd/alloy-tui` to support dynamic panes based on registered widgets.
