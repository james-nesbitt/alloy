# Alloy Agent & Development Guidelines

This document outlines the **mandatory** workflow for all agents (AI or human) contributing to this repository.

---

## 🚨 CRITICAL: POSITIVE CONTROL (MANDATORY) 🚨

These rules ensure repository integrity. **Failure to follow these rules is considered a critical error.**

### 1. The "No-Main" Branching Rule
*   **NEVER WORK ON MAIN**: All development **MUST** happen on a `feature/` or `fix/` branch.
*   **NEVER COMMIT TO MAIN**: You must not commit or push to `main` directly.

### 2. The "Stop-and-Wait" Merge Procedure
1.  **Work and Validate**: Complete the task on your branch. Run `just fmt` and `just test`.
2.  **Summary Phase**: Provide a concise summary of your changes and test results.
3.  **The Question**: Ask: **"Work is complete on branch `feature/xyz`. May I merge this into `main`?"**
4.  **Positive Affirmation**: You **MUST NOT** merge until you receive an explicit "Yes", "Merge it", or similar affirmative response. **Silence is NOT permission.**

---

## 🛠 Interaction Workflow

### 1. Read-Before-Write
Read existing documentation and relevant code before proposing changes to ensure consistency with Alloy's architecture.

### 2. Validation Suite
Before asking to merge, you must verify:
*   **Formatting**: `just fmt` passes.
*   **Compilation**: `just build-all` (Builds Core and WASM plugins).
*   **Testing**: `just test` passes (Functional and unit tests).

### 3. Documentation
Update the relevant files in `docs/` for any changes to architecture, protocol, or standards.

---

## 📁 Repository Reference

*   **`pkg/kernel/`**: Core Go-native services (IAM, KV, Events).
*   **`pkg/wasm/`**: WASM runtime and Go Guest SDK.
*   **`plugins/wasm/`**: Isolated application logic components.
*   **`api/`**: Shared IPC message schemas.
*   **`docs/`**: Deep-dive project documentation.
