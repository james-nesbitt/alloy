# ARCHITECT DESIGN: PHASE 7 (OMNI-PALETTE & WIDGET VIEWPORT)

## 1. Universal Omni-Palette (Search Routing)

The Omni-Palette is a **Push/Pull** hybrid interface that must feel instantaneous across all frontends.

### 1.1 Web: Async WebSocket Search
To avoid blocking the UI, the Web Frontend (`bridge.js`) will use a debounced WebSocket message pattern.

- **Sequence**:
    1.  **Typing**: UI captures `input` event, debounces (150ms).
    2.  **Request**: `bridge.js` sends `api.Message{ Method: "omni:search", Target: "omni-palette", ... }`.
    3.  **Kernel**: Routes to `omni-palette` (WASM).
    4.  **Plugin**: Performs search (Command Map, Indexer, Buffers).
    5.  **Response**: Plugin returns result list.
    6.  **Kernel**: Routes `api.TypeResponse` back to the specific `web-client` session.
    7.  **UI**: `bridge.js` handles the response and re-renders the `results` list dynamically.

### 1.2 TUI: Dedicated Palette View
The TUI will use a sub-model (`bubbles/list`) that is decoupled from the main editor/chat view.
- **Task Delegation**: The `ModalEngine` will produce a `SearchIntent`. The TUI `update()` will then transition to the `PaletteView`.

---

## 2. Dynamic Widget & Viewport System

Alloy uses a **Push-Update** model for its dashboard and status viewports.

### 2.1 Widget Message Protocol
A new event topic `dashboard:widget-updated` is used to synchronize UI components.

```json
{
    "id": "evt-12345",
    "type": "event",
    "sender": "plugin-id",
    "target": "events",
    "method": "publish",
    "payload": {
        "topic": "dashboard:widget-updated",
        "data": {
            "id": "widget-id",
            "title": "New Title",
            "content_type": "markdown",
            "content": "Updated Content Base64..."
        }
    }
}
```

### 2.2 Frontend Viewport Dispatch
- **Web**: Components with `data-widget-id="X"` will automatically update their inner HTML/Markdown when a matching event arrives.
- **TUI**: The TUI model will maintain a map of `WidgetID -> WidgetData`. When an event arrives, the model is updated, triggering a Bubble Tea `View()` refresh.

---

## 3. Project Auto-Discovery Architecture

The `project-manager` plugin uses a **Host-Privileged Mount** to scan for `.alloy` projects.

- **Mount Point**: The plugin is instantiated with a volume mount to the user's base data directory (e.g., `~/.config/alloy/projects`).
- **Scan Trigger**: Triggered on `Init()` and periodically via `system:tick`.
- **Registration**: For every discovered project, the plugin sends a `workspace:register` message to the **Kernel**.
- **Result**: The Kernel's workspace registry is populated without manual user entry.

---

## 4. Implementation Targets (PHASE 7)

1.  **Refactor `cmd/alloy-web`**:
    - Update `handlers.go` to ensure responses for WS requests are routed back to the correct connection. (Completed in Phase 6 Audit).
    - Update `bridge.js` for async search results.
2.  **Enhance `pkg/kernel`**:
    - Implement the `WidgetRegistry` in the kernel to track "All Registered Widgets" for new frontend joins.
    - Broadcast "Full Widget State" on new WebSocket connection.
