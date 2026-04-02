# Phase 11: Lifecycle & Audit

## Status: ACTIVE 🚀
**Goal**: Transition Alloy from a real-time collaboration tool into a persistent knowledge substrate with semantic memory and historical integrity.

---

### Workstream 0: Finishing Phase 10 (Dynamic Interactions) [DONE]
Before fully committing to Phase 11, we must close the remaining gaps in the "Dynamics" phase.

- [x] **Real-Time Cursor Synchronization**:
    - Update `plugins/wasm/buffer/main_wit.go` to emit `evt-cursor-updated` events when cursors move.
    - Update `cmd/alloy-tui`, `cmd/alloy-web` and `cmd/alloy-gui-gio` to render remote cursors in the viewport.
- [x] **Team Presence Enhancements**:
    - Integrate the `team-presence` activity stream into the Omni Palette search results (e.g., "See what others are working on").

### Workstream 1: The Librarian (Semantic Indexing) [DONE]
Provide the workspace with "Long-term Memory."

- [x] **Librarian Actor**: Implement a background Wasm actor that consumes workspace event streams.
- [x] **Local Vector Indexing**:
    - Integrate a lightweight vector store (linear KV scan for now) to index buffer content and chat history.
    - Implement `librarian:search` for fuzzy, multi-modal semantic search.
- [ ] **Context Injection**: Use the Librarian to provide "Context" for AI intents, automatically attaching relevant historical snippets to requests.

### Workstream 2: Temporal Playback (The Time Machine) [IN PROGRESS]
Move from current-state storage to event-sourced history.

- [x] **Event Store**: Implement a high-performance append-only log for all kernel messages (Audit Log V2).
- [x] **Scrubbing UI**: Add a "Time Machine" mode to the TUI/GUI that allows users to scrub back through project history.
- [ ] **Point-in-Time Recovery**: Allow restoring a project to any previous event ID.

### Workstream 3: Workspace Archival (.ark) [PLANNED]
Ensure project portability and longevity.

- [ ] **Archive Specification**: Define the `.ark` format as a self-contained bundle of project identity, state, and event history.
- [ ] **Cold Storage Logic**: Add `alloy:project/archive` and `alloy:project/restore` capabilities.
- [ ] **Migration Guard**: Tooling to verify archive integrity across Alloy versions.

---

## Technical Considerations

### Vector Storage in WASM
We will explore using a WASM-side memory-mapped index or delegating the heavy lifting to the Host via a `service:librarian-host` if WASM performance for vector math is insufficient.

### Event Sourcing Performance
The `RouteMessage` path must remain fast. The event store should be an asynchronous "Tee" from the main routing logic to avoid blocking real-time interactions.
