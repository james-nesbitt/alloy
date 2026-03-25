# AI Steering & Guidelines for Alloy

This document defines how AI agents must interact with the Alloy project. Adherence to these steering guidelines is **mandatory** to maintain architectural integrity and repository history.

---

## 🚨 CRITICAL: POSITIVE CONTROL (MANDATORY) 🚨

To prevent repository corruption and maintain high-quality history, agents must follow this exact Git workflow. There are **NO EXCEPTIONS**.

### 1. Branching Strategy
*   **DECOUPLED DEVELOPMENT**: All work **MUST** happen on a dedicated branch prefixed with `feature/` or `fix/` (e.g., `feature/wasm-mmap` or `fix/io-deadlock`).
*   **NEVER WORK ON MAIN**: If you find yourself on `main`, stop immediately, move changes to a new branch, and reset `main` to `origin/main`.

### 2. The Merge Procedure (The "Stop-and-Wait" Rule)
1.  **Work and Validate**: Complete the task on your branch. Run `just fmt` and `just test`.
2.  **Summary Phase**: Provide a concise summary of your changes, any test results, and any new architectural implications.
3.  **The Question**: Ask the user: **"Work is complete on branch `feature/xyz`. May I merge this into `main`?"**
4.  **Positive Affirmation**: You **MUST NOT** merge until you receive an explicit "Yes", "Merge it", "Proceed", or equivalent affirmative response. **Silence or moving to a new topic is NOT permission to merge.**

### 3. Commit Standards
*   Use descriptive, atomic commits.
*   Prefer the [conventional commits](https://www.conventionalcommits.org/) format (e.g., `feat(kernel): ...`, `fix(plugins): ...`).

---

## Design Philosophy

*   **Pragmatic Hybrid Kernel**: 
    *   **Core Services** (IAM, KV, Events, Telemetry) are integrated Go-native for performance.
    *   **Application Logic** (AI, Chat, Project Management) belongs in isolated WASM plugins.
*   **Message-Driven (IPC)**: All external interactions (Plugins and Frontends) occur over the asynchronous message bus.
*   **XDG Compliance**: Respect platform standards for file locations (`~/.config/alloy`, `~/.local/share/alloy`, etc.).

---

## Interaction Workflow for Agents

### 1. Read-Before-Write
Always read existing relevant files (horizontally) and documentation before proposing changes to ensure consistency with established patterns (e.g., how WASM host calls are implemented).

### 2. Validation Suite
Before asking to merge, you must verify:
*   **Formatting**: Code is formatted via `just fmt`.
*   **Compilation**: The project builds via `just build-all`.
*   **Testing**: All tests pass via `just test`. 
*   **Coverage**: Target 85% unit test coverage for new logic in `pkg/`.

### 3. Documentation Maintenance
Changes to architecture or core protocols **MUST** be reflected in the relevant `.md` files in the `docs/` directory during the same feature branch lifecycle.

---

## Project Directory Map

*   **`cmd/`**: Binary entry points (Alloy CLI, Core, TUI, GUI, Web).
*   **`pkg/kernel/`**: The core "Operating System" logic.
*   **`pkg/wasm/`**: WASM runtime and the SDK shared by all Go plugins.
*   **`plugins/wasm/`**: The individual WASM-based application components.
*   **`api/`**: Shared IPC message schemas.
*   **`wit/`**: WebAssembly Interface Type definitions.
*   **`docs/`**: Deep-dive architectural documentation.
