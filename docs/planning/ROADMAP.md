# ROADMAP: OPTIMAL OPERATIONS

## Phase Progress Summary

| Phase | status | Description |
| :--- | :--- | :--- |
| **PHASE 6: FOUNDATION** | **DONE** | Universal Modal Engine, Resource Isolation, Web Frontend. |
| **PHASE 7: ORCHESTRATION** | **DONE** | Manifest-based project orchestration & archetypes. |
| **PHASE 8: IAM & IDENTITY** | **DONE** | Hierarchy-Aware RBAC, Role Attestation, Scoped Discovery. |
| **PHASE 9: SYNTHESIS** | **DONE** | Multi-Project Switcher, Side-car plugins. |
| **PHASE 10: DYNAMICS** | **ARCHIVED** | Layout recursion, Intent routing, Headless actors. |
| **PHASE 11: LIFECYCLE** | **DONE** | Semantic indexing (Librarian), temporal playback. |
| **PHASE 12: SUBSTRATE** | **DONE** | Agentic Actor Framework, Headless Coordination. |
| **PHASE 13: ORCHESTRATION** | **DONE** | Dynamic Manifests, Simple Project Handling. |
| **PHASE 14: PROJECT EDITOR** | **ACTIVE** | Context-aware editing, LSP, Multi-buffer sync, Git. |
| **PHASE 15: AUTONOMOUS II** | **PLANNED** | Machine-First Coordination, Zero-Trust Bridge. |

---

## Current Active: PHASE 14: ALLOY AS A PROJECT EDITOR [IN PROGRESS]

**Goal**: Transform Alloy from a plugin host into a first-class development environment. This phase focuses on the "Edit-Test-Commit" cycle, leveraging AI actors and high-fidelity project context.

### Phase 14 Milestones
- [ ] **High-Fidelity Context Engine**
    - LSP integration (gopls, rust-analyzer) wrapping via WASM/Go plugins.
    - Semantic "Context Windows" for AI actors based on active buffers and symbol graphs.
- [ ] **Multi-Buffer Coordination**
    - Atomic edits across multiple files (refactoring).
    - Virtual Buffers for generated code and "Draft View" exploration.
- [ ] **SCM-First Workflow**
    - Native Git integration (diffing, staging, branch management) as first-class intents.
    - Attribution-aware commits (tracking which agent/user made which change).
- [ ] **Integrated Pipeline**
    - Running builds, linters, and tests as background activities with real-time feedback loops.

---

## Next: PHASE 15: THE AUTONOMOUS SUBSTRATE II
- **Fully Decoupled Frontends**: Any device can render the workspace intent.
- **Machine-First Coordination**: AI actors participate as first-class citizens in the team roles.
- **Zero-Trust Collaboration**: Hardware-verified identity and isolation at every layer.
