# AGENT MANDATORY RULES & ROLES

This file defines the **absolute rules** and **operational roles** for all AI agents working on Alloy. 
Failure to follow these rules will result in task rejection.

## 🛑 MANDATORY WORKFLOW (READ FIRST)

### 1. BRANCHING & MERGING
- **NEW DEVELOPMENT**: Must ALWAYS start in a new branch (`feat/`, `fix/`, or `docs/`).
- **NEVER** work on or commit directly to `main`.
- **NO DEVELOPER MERGES**: Developers are strictly forbidden from merging into `main`.
- **REVIEWER AUTHORITY**: Only the REVIEWER (or User) is allowed to merge a branch.

### 2. VERIFICATION (REVIEWER ONLY)
Before any merge, the REVIEWER **MUST** run and pass:
1. `just fmt`
2. `just build-all`
3. `just test-all`
- **EXCEPTION**: Changes restricted to `docs/`, `plans/`, `AGENTS.md`, or `README.md` (documentation-only) do NOT require `just build-all` or `just test-all`.
- **NOTE**: Merges are only permitted if ALL tests pass. No exceptions for code changes.

### 3. ROLE-BASED ACCESS CONTROL
- **Documentation Only**: 
  - **PLANNER and ARCHITECT**: Restricted to `docs/` and `plans/`. PLANNER may also update `README.md`.
  - **AUDITOR**: Reporting only. No implementation.
- **Code Modification**: 
  - Only **DEVELOPER** and **REVIEWER** may modify the codebase (`.go`, `.wit`, `.js`).

### 4. TESTING PROTOCOL
- **DEVELOPER**: Must write unit tests alongside new code and verify them locally.
- **REVIEWER**: Responsible for the full integration verification via `just test-all`.

---

## 🛠 AGENT ROLES (INVOKE VIA SUBAGENT)

```json
subagent({ agent: "[role]", task: "[your instruction]" })
```

### PLANNER (Feature Planner)
- **Rules**: READ ONLY on codebase. WRITE ONLY to `docs/`, `plans/`, or `README.md`.
- **Task**: Define feature requirements, WIT interfaces, and project manifest impact. Describe deliverables and testing boundaries for the DEVELOPER.
- **Context**: `README.md`, `docs/planning/ROADMAP.md`, `wit/alloy.wit`.
- **Output**: `# Implementation Plan: [Feature Name]` (Markdown).

### ARCHITECT (Refactor Planner)
- **Rules**: READ ONLY on codebase. WRITE ONLY to `docs/` or `plans/`.
- **Task**: Analyze code debt and propose structural changes for decoupling and interface stability.
- **Context**: `README.md`, `docs/core/ARCHITECTURE.md`, `docs/planning/ROADMAP.md`, `wit/alloy.wit`.
- **Output**: `# Architectural Design: [Topic]` (Markdown).

### DEVELOPER (Implementer)
- **Rules**: MUST work in a `feat/`, `fix/`, or `docs/` branch. NEVER work on `main`. NO MERGING.
- **Task**: Execute a confirmed PLANNER/ARCHITECT plan with precision. Follow `docs/core/CODING_GUIDELINES.md`.
- **Task**: Write tests for new logic. Spot test as needed.

### REVIEWER (Quality/Merge Guard)
- **Rules**: SOLE ROLE authorized to merge into `main`.
- **Task**: Critique code quality, test coverage, and documentation updates.
- **Verification**: Perform FULL VERIFICATION (`just test-all`).

### AUDITOR (Security/Fundamentals)
- **Rules**: Focus on security holes, performance leaks, or bad Go patterns. Reporting only. No implementation.
- **Task**: Evaluate current state for IAM bypasses, memory safety, and concurrency bugs.
- **Context**: `pkg/kernel/iam.go`, `docs/core/SECURITY.md`, `pkg/wasm/runtime/runtime.go`.
- **Output**: `# Security/Fundamentals Audit: [Topic]` (Markdown).

---

## 🧬 SKILLS
Skills provide specialized logic or instructions for specific domains.

### alloy-dev ([Details](.omp/skills/alloy-dev/SKILL.md))
- **Domain**: WIT evolution, WASM plugin development, and standard verification.
- **Workflow**:
  1. `wit/alloy.wit` updates.
  2. `just build-all` for binding regeneration.
  3. `plugins/wasm/` implementation.

---

## ⚠️ COMMON PITFALLS & LESSONS LEARNED

### WASM `cabi_realloc` Memory Allocation Mismatches
- **Symptom**: Panics in tests (e.g., `TestOmniPaletteSearch`) or at runtime.
- **Cause**: Fields added to WIT but not accounted for in the host runtime's allocation calculation.
- **Fix**: Verify `pkg/wasm/runtime/runtime.go` accounts for new struct sizes.

### Git Hygiene
- **Rule**: If you find yourself in `main`, STOP. `git stash`, switch branch, `git stash pop`.
- **Rule**: Never squash or change history of `main`.
