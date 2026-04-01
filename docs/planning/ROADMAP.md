# ROADMAP: OPTIMAL OPERATIONS

## Phase Progress Summary

| Phase | status | Description |
| :--- | :--- | :--- |
| **PHASE 6: FOUNDATION** | **DONE** | Universal Modal Engine, Resource Isolation, Web Frontend. |
| **PHASE 7: ORCHESTRATION** | **DONE** | Manifest-based project orchestration & archetypes. |
| **PHASE 8: IAM & IDENTITY** | **DONE** | Hierarchy-Aware RBAC, Role Attestation, Scoped Discovery. |
| **PHASE 9: SYNTHESIS** | **DONE** | Multi-Project Switcher, Side-car plugins. |
| **PHASE 10: DYNAMICS** | **IN PROGRESS** | Layout recursion, Intent routing, Headless actors. |
| **PHASE 11: LIFECYCLE** | **PLANNED** | Semantic indexing (Librarian), temporal playback. |

---

## Current Active: PHASE 10: ADVANCED DYNAMICS

**Goal**: Transform Alloy into an intent-driven substrate with unified layout orchestration and autonomous actor support.

- [x] **Recursive Trinity Layout**: Universal split rendering with nested horizontal/vertical splits.
- [x] **Workspace View Persistence**: Store/Restore last known good layout via the project plugin.
- [ ] **Intent-Based Routing V1**:
    - [ ] `alloy:intent/intent.wit` definition.
    - [ ] Intent discovery via `alloy_capability_t`.
    - [ ] Kernel-level `intent_broker.go` for fuzzy goal-oriented routing.
- [ ] **Headless Actor Framework**:
    - [ ] Background lifecycle for "Invisible" plugins (Actors).
    - [ ] Service Discovery V2 for background services (e.g. `service:code-analysis`).
- [ ] **Presence & Activity Sync**:
    - [ ] Real-time cursor/selection synchronization across frontends.
    - [ ] Activity stream integration for "Team Presence".

---

## Upcoming: PHASE 11: LIFECYCLE & AUDIT

**Goal**: Move from real-time coordination to semantic knowledge management and historical integrity.

- [ ] **The Librarian Service (Semantic Indexing)**
    - Long-term memory for the workspace.
    - Local vector store for indexing chat, logs, and buffer history.
    - Cross-project semantic search.
- [ ] **Temporal Playback (Event History)**
    - The "Time Machine" for the workspace.
    - Replay event streams to reconstruct previous states.
    - Audit-grade event log persistence.
- [ ] **Workspace Archival (`.ark`)**
    - Package a project, its state, and its history into a single portable archive.
    - Support for "Cold Storage" and project handovers.

---

## Long-term Vision: THE AUTONOMOUS SUBSTRATE
- **Fully Decoupled Frontends**: Any device can render the workspace intent.
- **Machine-First Coordination**: AI actors participate as first-class citizens in the team roles.
- **Zero-Trust Collaboration**: Hardware-verified identity and isolation at every layer.
