# Phase 14: Alloy as a Project Editor

**Status**: PLANNING
**Date**: 2026-04-06  
**Goal**: Transform Alloy from a general-purpose plugin host into a high-fidelity, autonomous-first code editor. This phase focuses on the "Edit-Test-Commit" cycle and deep integration with language servers and existing SCM tools.

---

## 1. Core Principles (Phase 14)

### 1.1 High-Fidelity Context Engine (Principle #1)
The editor must be aware of the codebase's semantics, not just its text. This requires deep LSP integration and semantic indexing.

### 1.2 Multi-Buffer Coordination (Principle #2)
Complex refactoring and broad-scale edits must have transactional safety across multiple buffers.

### 1.3 SCM-First Workflow (Principle #3)
Git/Mercurial integration must be a first-class citizen of the interaction loop, not an external afterthought.

---

## 2. Technical Milestones (Phase 14)

### Milestone 1: LSP Integration (gopls/rust-analyzer)
- [ ] **LSP Adapter Plugin**: Create a WASM plugin that bridges to a host-side LSP server (e.g. `gopls`).
- [ ] **Capability**: Advertise `editor:symbols` and `editor:references`.
- [ ] **Implementation**:
    1.  `lsp:request_definition(context, buffer_id, offset)`
    2.  `lsp:request_references(context, buffer_id, offset)`
    3.  `lsp:get_diagnostics(context, buffer_id)`

### Milestone 2: Native Git Integration
- [ ] **Git Plugin**: Wrap `go-git` or similar in a WASM plugin.
- [ ] **Capability**: Advertise `scm:git`.
- [ ] **Implementation**:
    1.  `git:diff(context, path)`
    2.  `git:stage(context, path)`
    3.  `git:commit(context, message)`

### Milestone 3: Integrated Build & Test Pipeline
- [ ] **Pipeline Plugin**: Listens for buffer updates and triggers background builds.
- [ ] **Implementation**:
    1.  `pipeline:run_task(context, task_id)`
    2.  Stream output to virtual buffer (e.g., `buffer:output-build`).

---

## 3. Phase 13 Summary (DONE 🚀)

### Completed Milestones
- [x] **Metadata-Only Registry**: The Kernel now registers plugins without instantiating them, checking manifests for capabilities.
- [x] **Filesystem Discovery Plugin**: Implemented a WASM plugin that discovers Bases at a given path.
- [x] **Discovery Delegation**: `BaseManager` successfully delegates base discovery to the `filesystem` plugin.
- [x] **Base-Scoping Isolation**: IAM now enforces isolation between different project Bases.
- [x] **Just develop**: Automated project-centric launch and TUI activation.

### Key Decisions
- **Capability-Led Bootstrap**: The system is now truly agnostic to the filesystem; it just triggers capabilities.
- **WASM Guest Mounts**: Settled on using directory-based mounts via wazero for plugin isolation.

---

## 4. AMOP Compliance
- **Worktree**: `/var/home/jnesbitt/Documents/Personal/alloy/.worktrees/phase-14-project-editor`
- **Branch**: `feat/phase-14-project-editor`
- **Verification**: `just build-all` + `just develop`.
