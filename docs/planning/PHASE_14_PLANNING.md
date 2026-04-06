# Phase 14 Implementation: Alloy as a Project Editor

Status: ACTIVE 🚀
Date: 2026-04-06

**Goal**: Transform Alloy from a general-purpose plugin host into a high-fidelity, autonomous-first code editor. This phase focuses on the "Edit-Test-Commit" cycle and deep integration with language servers and existing SCM tools.

---

## 1. High-Fidelity Context Engine (HCE)
Make the editor aware of the codebase's semantics, not just its text.

- [ ] **LSP Implementation (LSPi)**:
    - Implement an LSP client in the Kernel or a dedicated `lsp-adapter` plugin.
    - Bridge `gopls`, `rust-analyzer`, etc., via WASM-accessible IPC or host-side sidecars.
    - Standardize the `editor:symbols` and `editor:references` intents for cross-plugin usage.
- [ ] **Semantic Context for AI**:
    - Build "Context Windows" by combining active buffer text with related symbol definitions (from LSP).
    - Provide agents with "Intent-Aware Grapples" to browse code structurally rather than file-by-file.

## 2. Multi-Buffer Coordination (MBC)
Enable complex refactoring and broad-scale edits with transactional safety.

- [ ] **Transactional Multi-Edit**:
    - Allow an actor to propose a set of changes across multiple buffers.
    - Implement a "Preview Mode" (Draft View) where all changes are staged but not committed to the underlying filesystem.
- [ ] **Virtual/Generated Buffers**:
    - Support for volatile buffers that contain generated output (e.g., protobuf code, mock data) that syncs instantly with source changes.

## 3. SCM-First Workflow (SCM)
Bring Git/Mercurial integration into the core interaction loop.

- [ ] **Native Git Plugin**:
    - Wrap `go-git` or similar in a WASM plugin.
    - Implement `git:diff`, `git:stage`, `git:commit`, and `git:branch` intents.
    - Expose "Team View" in the UI (Presence + Git branch state).
- [ ] **Attribution-Aware Commit**:
    - Capture which actor (AI or Human) authored each chunk of code in a commit.
    - Cross-reference commit IDs with the Kernel History (Phase 11) for full auditability.

## 4. Integrated Pipeline (IPL)
Automate the verification of changes as they happen.

- [ ] **Background Verification**:
    - Create a `pipeline` plugin that listens for buffer updates and triggers builds/tests.
    - Stream build output into a virtual buffer for real-time inspection.
- [ ] **Auto-Fix Loop**:
    - Let agents automatically react to linter/compiler errors by analyzing the diagnostic feed from the pipeline.

---

## Technical Constraints & Design Principles

- **Zero-Latency Response**: Large file editing must remain fluid in the TUI/GUI, with heavy lifting (LSP/Indexing) moved to background threads.
- **FS-Symmetry**: Changes in Alloy must be instantly visible to external tools (just/make/shell), and vice-versa.
- **Substrate Neutrality**: The editor features must work identically whether running on a local TUI or a remote headless core.

---

## Branch Strategy

- All work will occur in `feat/phase-14-project-editor` or sub-branches (`feat/p14-lsp-integration`, etc.).
- `main` remains the stable integrated base.
