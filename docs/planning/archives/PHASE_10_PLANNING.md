# Phase 10: Advanced Dynamics

## Status: PLANNED 🚀
**Goal**: Transform Alloy into an intent-driven substrate with unified layout orchestration and autonomous actor support.

---

### Workstream 1: Unified Layout & View State
Refactor the fragile, duplicated frontend layout logic into a single source of truth.

- [ ] **Recursive Layout Schema**: Update `api.WorkspaceConfig` to support nested horizontal/vertical splits and `Slot` bindings.
- [ ] **State Persistence**: Update the `project` plugin to store "Last Known Good Layout" in the KV store.
- [ ] **Shared Handler**: Refactor `cmd/alloy-tui` and `cmd/alloy-gui-gio` to use a common layout application loop.

### Workstream 2: Intent-Driven Routing (V1)
Move from rigid RPC calls to fuzzy, goal-oriented "Intents".

- [ ] **Intent WIT**: Define `alloy:intent/intent.wit` for semantic routing.
- [ ] **Metadata Enrichment**: Add `Intents []string` to `alloy_capability_t` so plugins can declare what intents they satisfy.
- [ ] **The Broker**: Implement `pkg/kernel/intent_broker.go` to route intents based on plugin registration and priority.

### Workstream 3: User Personalization & Sidecars
Make the "Sidecar" experience first-class.

- [ ] **Settings Plugin**: A WASM plugin to manage `user-config.json` via a UI.
- [ ] **Global Injection**: Ensure plugins in `~/.alloy/sidecars` have correctly scoped "Omni-Permissions".
- [ ] **Theme Sync**: Propagate user theme choices across all Frontends and WASM widgets.

### Workstream 4: Headless Actors
Support background automation.

- [ ] **Background Lifecycle**: Update `pkg/wasm/manager.go` to support "Invisible" plugins (Actors).
- [ ] **Service Discovery V2**: Allow actors to register as specific "Services" (e.g., `service:code-analysis`) separate from UI widgets.

---

## Technical Details

### Proposed `WorkspaceLayout` Schema
```json
{
  "root": {
    "type": "split",
    "direction": "horizontal",
    "children": [
      { "type": "pane", "plugin": "buffer", "width": 0.7 },
      { 
        "type": "split", 
        "direction": "vertical", 
        "width": 0.3,
        "children": [
           { "type": "pane", "plugin": "chat", "height": 0.5 },
           { "type": "pane", "plugin": "ai", "height": 0.5 }
        ]
      }
    ]
  }
}
```

### Intent Example
```go
// Instead of:
client.Send("buffer", "write", payload)

// Use:
client.DispatchIntent("intent:save", payload)
// ...which routes to the active buffer, or a cloud-sync plugin, or the local FS.
```
