# Phase 13: Capability-Led Base Refactor

**Status**: PLANNING (REFINED)
**Date**: 2026-04-06  
**Goal**: Transform the Kernel into an environment-agnostic "Base Host" that delegates discovery and initialization to plugins via **structured capabilities**.

---

## 1. Core Principles (Refined)

### 1.1 Capability-Led Bootstrap (Principle #1)
The Kernel must not contain any hardcoded logic for discovering a "Base" (project/workspace). It only knows how to:
1.  **Register** available plugins (reading metadata without instantiating WASM).
2.  **Match** a requested "Discovery Capability" to a registered plugin.
3.  **Delegate** the discovery process to that plugin.

### 1.2 FS-Independence (Principle #2)
The Kernel's only native filesystem operations are:
1.  Scanning a **Global Plugin Directory** for WASM binaries + manifests.
2.  Providing a **WASI-mapped mount** to plugins that require disk access (optional).
*Config discovery, project scanning, and manifest generation must happen inside WASM plugins.*

### 1.3 Advocacy & Suggestions (Principle #3)
The CLI or Frontend (the "Advocate") suggests which plugins to use for discovery based on its environment:
-   **CLI**: Suggests `filesystem-provider` when running `alloy .`.
-   **Web Frontend**: Suggests `remote-provider` or `indexed-db-provider`.

---

## 2. Technical Milestones (Revised)

### Milestone 1: Metadata-Only Registry & Structured Capabilities
- [ ] **Plugin Metadata**: Update `api/messages.go` and the Plugin Manifest format to include `Advertisements` (e.g., `{ "type": "base:provider", "variant": "path" }`).
- [ ] **Lazy Registration**: Refactor `pkg/kernel/registry.go` to register plugins by reading their `.json` or `.wit` headers *without* starting the WASM guest.
- [ ] **Capability Matching**: Implement a lookup engine in the Kernel to find plugins by advertised capability.

### Milestone 2: The "Filesystem" Discovery Plugin (WASM)
- [ ] Create a minimalist `filesystem` WASM plugin (independent of the `project` plugin).
- [ ] **Capability**: Advertise `base:provider:path`.
- [ ] **Logic**: Implements `base:discover(context) -> BaseManifest`.
- [ ] **WASI Usage**: Reads `.alloy/plugins/` using the standard WASI filesystem API (mapped by the Kernel).

### Milestone 3: Discovery Delegation (Base Manager)
- [ ] **Refactor BaseManager**: Remove all `os` and `filepath` references.
- [ ] **Implementation**:
    1.  `BaseManager:activate(context, req_capability)`
    2.  Find plugin matching `req_capability`.
    3.  Instantiate and send `base:discover(context)`.
    4.  Receive `BaseManifest` and realize the described ecosystem.

### Milestone 4: Multi-Base Isolation (IAM)
- [x] **Mandatory Base-Scoping**: Ensure the `IdentityManager` isolates messages by `BaseID`. (Completed in Prelim M1).
- [ ] **Instance Multiplexing**: Support multiple WASM guest instances for the same plugin binary, keyed by `BaseID`.

---

## 3. Workflow Example: `alloy .`

1.  **CLI** starts and connects to the Kernel.
2.  **CLI** sends: `kernel:activate(context={"path": "."}, capability="base:provider:path")`.
3.  **Kernel** checks its registry for a plugin advertising `base:provider:path`.
4.  **Kernel** finds `plugins/filesystem.wasm`.
5.  **Kernel** instantiates `filesystem` and maps the current directory to the guest's `/work` mount.
6.  **Kernel** sends `base:discover(context={"root": "/work"})` to the `filesystem` guest.
7.  **Guest** returns a manifest: `{"plugins": {"chat": {...}, "ai": {...}}}`.
8.  **Kernel** loads `chat` and `ai` plugins, injecting their base-specific configs.

---

## 4. Progress Tracking

### Milestone 1: In Progress
- [x] API Definitions (`BaseID`, `InstancePattern`)
- [x] IAM Isolation (Base-Scoping)
- [ ] Structured Capability Advertisements
- [ ] Metadata-Only Registration Loop

### Milestone 2: Pending
- [ ] `filesystem` plugin implementation

---

## 5. AMOP Compliance
- **Worktree**: `/var/home/jnesbitt/Documents/Personal/alloy/.worktrees/phase-13-path-init`
- **Branch**: `phase-13-path-init`
- **Verification**: `just build-all` + unit tests for each milestone.
