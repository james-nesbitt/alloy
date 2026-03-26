# Alloy Agent Development Guidelines

This document outlines the **mandatory** workflow for all AI and human agents contributing to the Alloy repository. These rules are designed to maintain repository integrity, ensure positive control, and prevent "hallucination-driven" regressions.

---

## 🚨 CRITICAL: POSITIVE CONTROL (MANDATORY) 🚨

The following rules are non-negotiable. **Any deviation is a critical operational failure.**

### 1. The Branching Requirement
*   **NEVER WORK ON MAIN**: All development **MUST** happen on a dedicated branch prefixed with `feat/`, `fix/`, or `docs/`.
*   **NEVER COMMIT TO MAIN**: Directly pushing or committing to the `main` branch is strictly prohibited.

### 2. The Verification Suite
Before any merge can be considered, the following **MUST** be executed and pass in the current environment:
*   `just fmt`: All source code must be formatted.
*   `just build-all`: Every component (Core, Plugins, Frontends) must compile.
*   `just test`: Every unit and integration test must pass.

### 3. The Merge & Push Protocol
You may only merge a branch into `main` and push to the remote if **one** of the following conditions is met:

#### Condition A: Explicit Human Affirmation
1.  Provide a concise summary of the changes and the successful `just test` results.
2.  Ask: **"Work is complete on branch `xyz`. May I merge this into `main` and push?"**
3.  Receive an **explicit "Yes" or "Merge it"** from the human user. Silence is not permission.

#### Condition B: Full Automated Validation
1.  You have successfully executed `just fmt`, `just build-all`, and `just test` in the terminal during the current session.
2.  All tests passed with zero failures.
3.  The user has previously authorized "Test-Driven Autonomy" for the current task.

---

## 🛠 Operational Workflow (The "Loop")

### 1. Contextual Loading (Read-Before-Write)
Before proposing or implementing ANY change:
*   **Read the Roadmap**: Ensure the change aligns with the current Phase in `docs/ROADMAP.md`.
*   **Read the Architecture**: Verify the change respects the "Coordination Kernel" model in `docs/ARCHITECTURE.md`.
*   **Scan the Bindings**: If changing plugin logic, review `wit/alloy.wit` to ensure interface compatibility.

### 2. Incremental Commits
Do not wait until the end of a task to commit. Commit logical milestones frequently with descriptive messages. This allows for easier "rewinds" if a technical path proves non-viable.

### 3. Documentation Parity
Any change to core logic, plugin interfaces, or coordination patterns **MUST** be accompanied by a corresponding update in the `docs/` directory within the same branch.

---

## 📁 Critical File Manifest

*   **`pkg/kernel/`**: Native coordination services (Security, Events, Registry).
*   **`pkg/wasm/`**: Plugin runtime and the WIT-based Host/Guest bridge.
*   **`plugins/wasm/`**: Application-specific "Shared Effort" components.
*   **`api/`**: Kernel↔Frontend message schemas.
*   **`docs/`**: The authoritative source for system goals and constraints.
