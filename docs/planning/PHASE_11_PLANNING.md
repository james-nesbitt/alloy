# Phase 11: Lifecycle & Audit

## Status: PLANNED 🚀
**Goal**: Transition Alloy from a real-time collaboration tool into a persistent knowledge substrate with semantic memory and historical integrity.

---

### Workstream 0: Finishing Phase 10 (Dynamic Interactions)
Before fully committing to Phase 11, we must close the remaining gaps in the "Dynamics" phase.

- [ ] **Real-Time Cursor Synchronization**:
    - Update `plugins/wasm/buffer/main_wit.go` to emit `evt-cursor-updated` events when cursors move.
    - Update `cmd/alloy-tui` and `cmd/alloy-web` to render remote cursors in the viewport.
- [ ] **Team Presence Enhancements**:
    - Integrate the `team-presence` activity stream into the Omni Palette search results (e.g., "See what others are working on").

### Workstream 1: The Librarian (Semantic Indexing)
Provide the workspace with "Long-term Memory."

- [ ] **Librarian Actor**: Implement a background Wasm actor that consumes workspace event streams.
- [ ] **Local Vector Indexing**:
    - Integrate a lightweight vector store (e.g., `bleve` or a simpler KV-based embedding index) to index buffer content and chat history.
    - Implement `intent:search/semantic` for fuzzy, multi-modal search.
- [ ] **Context Injection**: Use the Librarian to provide "Context" for AI intents, automatically attaching relevant historical snippets to requests.

### Workstream 2: Temporal Playback (The Time Machine)
Move from current-state storage to event-sourced history.

- [ ] **Event Store**: Implement a high-performance append-only log for all kernel messages (Audit Log V2).
- [ ] **Scrubbing UI**: Add a "Time Machine" mode to the TUI/GUI that allows users to scrub back through project history.
- [ ] **Point-in-Time Recovery**: Allow restoring a project to any previous event ID.

### Workstream 3: Workspace Archival (.ark)
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
