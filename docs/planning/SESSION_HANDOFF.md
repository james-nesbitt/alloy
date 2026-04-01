# Session Handoff: Transition to Phase 11

## Current State
- **Phase 10 (Advanced Dynamics)**: Core implementation completed and merged into `main`.
- **Critical Fix**: Resolved a WASM ABI mismatch in `pkg/wasm/runtime/runtime.go` where `cabi_realloc` was under-allocating memory for `AlloyCapability` structs (52 vs 40 bytes) due to the new `Intents` field. This fixed the `TestOmniPaletteSearch` panics.
- **TUI Cursor Sync**: Initial implementation done. TUI now renders remote cursors in the status bar (User + Position).

## Branch Information
- **`main`**: Contains the stable Phase 10 implementation.
- **`feat/phase-11-planning`**: (Current) Contains updated `ROADMAP.md`, `README.md`, and the new `PHASE_11_PLANNING.md`.

## Immediate Next Steps (Workstream 0: Dynamics Cleanup)
1. **Real-Time Cursor Sync (Web/GUI)**:
   - Update `cmd/alloy-web` and `cmd/alloy-gui-gio` to render remote cursors.
   - The TUI currently fetches buffer content on `buffer:cursors_updated` event—this works but is slightly heavy. Consider a more granular `buffer:get_cursors` method if performance becomes an issue.
2. **Presence Activity**:
   - Update `plugins/wasm/omni-palette/main_wit.go` to include a "Team Activity" section in search results, querying the `team-presence` plugin.

## Phase 11 Implementation (Lifecycle & Audit)
1. **Librarian Service**: Needs a vector store strategy for WASM. (See `docs/planning/PHASE_11_PLANNING.md`).
2. **Temporal Playback**: Event sourcing for the kernel.
3. **Workspace Archival**: `.ark` format definition.

## Impediments
- None currently. The ABI bug was the major blocker and it is resolved.

## Deployment/Audit Notes
- `AI_GUIDELINES.md` was updated with a mandatory lesson regarding `cabi_realloc` size calculations when WIT interfaces change.
