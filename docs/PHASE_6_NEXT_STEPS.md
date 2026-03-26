# PLAN: PHASE 6 COMPLETION (FOUNDATION)

## 1. Universal Modal Engine: Intent-Based Migration
Currently, `alloy-tui` uses the `ModalEngine` as a "passive" agent that only switches modes. To achieve the roadmap goals, we must move to **100% Intent Execution**.

### 1.1 Intent Execution Mapping
We need to create a `pkg/frontend/tui/actions.go` (or similar) that maps modal intents to TUI model updates.
- **`MoveIntent`** -> Update `m.viewport` or `m.textarea` cursor.
- **`ActionIntent`** -> Trigger `chat:send`, `buffer:write`, or common commands.
- **`SearchIntent`** -> Activate the `Omni-Palette`.

### 1.2 Multi-Key Sequence Support
Expand the `ModalEngine` to handle multi-key sequences (e.g., `gg`, `dd`, `yw`) by maintaining a `pending` buffer in the `State`.

---

## 2. Resource Isolation (WASM Sandbox Hardening)
To ensure the **Coordination Kernel** is resilient, we must protect it from misbehaving or greedy plugins.

### 2.1 Memory Hardening
Map configuration (Initial/Max Memory) from kernel flags or the `alloy-project.json` manifest directly to the `wazero.ModuleConfig`.
- **Goal**: Prevent a single plugin from consuming all system RAM.

### 2.2 CPU/Instruction "Fuel"
Implement `wazero`'s instruction counting ("fuel") mechanism in `pkg/wasm/runtime/runtime.go`.
- **Mechanism**: Every plugin call is allocated a fixed amount of "fuel" (instructions). If the plugin exceeds this during a single call (e.g., an infinite loop), the kernel terminates the call and logs a `plugin:crashed` event.

---

## 3. Alloy Web: The Browser Gateway
Research the implementation of a Go-WASM client that can serve as the **Web Frontend**.

### 3.1 IPC over WebSockets
Define a standardized `api.Message` transport that works over persistent WebSocket connections for the browser.

### 3.2 Viewport Composition
Plan how the **Workspace Composition Engine** will render on the web (e.g., using React/Vue components that speak the `api.Message` protocol).

---

## Progress: PHASE 6 COMPLETION (FOUNDATION)

### Completed Tasks:
- [x] **Modal Engine Logic**: Added multi-key buffer and count support to `ModalEngine`.
- [x] **Kernel Hardening**: Updated `pkg/wasm/runtime` to enforce strict memory limits and execution fuel via context-timed proxies.
- [x] **Helix Modal Driver**: Implemented a selection-first Helix driver with `g` and `z` sequences for modal diversity.
- [x] **WebSocket Migration**: Transitioned `alloy-web` from SSE/POST to unified IPC over WebSockets.

---

## 5. Next Implementation Step (DEVELOPER)
1.  **Omni-Palette Integration**: Integrate the `Omni-Palette` plugin's search results directly into the `alloy-web` search UI via the new WebSocket connection.
2.  **Viewport Composition**: Implement the first "Widget" rendering on the web dashboard using the `alloy-web` static assets.
