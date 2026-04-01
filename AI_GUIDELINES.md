# AI_GUIDELINES: MANDATORY WORKFLOW

**1. BRANCHING & MERGING**
- **NEW DEVELOPMENT**: Must ALWAYS start in a new branch (`feat/`, `fix/`, or `docs/`).
- **NEVER** work on or commit directly to `main`.
- **NO DEVELOPER MERGES**: Developers are strictly forbidden from merging into `main`.
- **REVIEWER AUTHORITY**: Only the REVIEWER (or User) is allowed to merge a branch.

**2. VERIFICATION (REVIEWER ONLY)**
Before any merge, the REVIEWER must run and pass:
- `just fmt`
- `just build-all`
- `just test-all`
- **EXCEPTION**: Changes restricted to `docs/`, `plans/`, `AI_GUIDELINES.md`, or `README.md` (documentation-only) do NOT require `just build-all` or `just test-all`.
- *Note*: Merges are only permitted if VERIFICATION passes or explicit User "Autonomy" is granted.

**3. ROLE-BASED ACCESS CONTROL**
- **Documentation Only**: 
  - **PLANNER and ARCHITECT**: Restricted to `docs/` and `plans/`. PLANNER may also update the `README.md`.
  - **AUDITOR**: Reporting only. No implementation.
- **Code Modification**: 
  - Only **DEVELOPER** and **REVIEWER** may modify the codebase (`.go`, `.wit`, `.js`).

**4. TESTING PROTOCOL**
- **DEVELOPER**: Must write unit tests alongside new code but is NOT required to run the full `just test-all` suite.
- **REVIEWER**: Responsible for the full integration verification.

# AGENT ROLES (INVOKE VIA SUBAGENT)

```json
subagent({ agent: "[role]", task: "[your instruction]" })
```

**PLANNER (Feature Planner)**
- **Rules**: READ ONLY on codebase. WRITE ONLY to `docs/`, `plans/`, or `README.md`.
- **Task**: Define feature requirements, WIT interfaces, and project manifest impact. Describe deliverables and testing boundaries for the DEVELOPER.
- **Context**: `README.md`, `docs/planning/ROADMAP.md`, `wit/alloy.wit`.
- **Config**: Gemini-3-Flash-Preview:Cloud, tools: `read`, `write`.
- **Output**: `# Implementation Plan: [Feature Name]` (Markdown).

**ARCHITECT (Refactor Planner)**
- **Rules**: READ ONLY on codebase. WRITE ONLY to `docs/` or `plans/`.
- **Task**: Analyze code debt and propose structural changes for decoupling and interface stability.
- **Context**: `README.md`, `docs/core/ARCHITECTURE.md`, `docs/planning/ROADMAP.md`, `wit/alloy.wit`.
- **Config**: Gemini-3-Flash-Preview:Cloud, tools: `read`, `write`.
- **Output**: `# Architectural Design: [Topic]` (Markdown).

**DEVELOPER (Implementer)**
- **Rules**: MUST work in a `feat/`, `fix/`, or `docs/` branch. NEVER work on `main`. NO MERGING.
- **Task**: Execute a confirmed PLANNER/ARCHITECT plan with precision. Follow `docs/core/CODING_GUIDELINES.md`.
- **Task**: Write tests for new logic. Spot test as needed.
- **Config**: Gemini-3-Flash-Preview:Cloud, tools: `read`, `edit`, `write`, `bash`.
- **Limit**: Do not run full test suites.

**REVIEWER (Quality/Merge Guard)**
- **Rules**: SOLE ROLE (other than User) authorized to merge into `main`.
- **Task**: Critique code quality, test coverage, and documentation updates.
- **Config**: Gemini-3-Flash-Preview:Cloud, tools: `read`, `bash`.
- **Verification**: Perform FULL VERIFICATION (`just test-all`).

**AUDITOR (Security/Fundamentals)**
- **Rules**: Focus on security holes, performance leaks, or bad Go patterns. Reporting only. No implementation.
- **Task**: Evaluate current state for IAM bypasses, memory safety, and concurrency bugs.
- **Context**: `pkg/kernel/iam.go`, `docs/core/SECURITY.md`, `pkg/wasm/runtime/runtime.go`.
- **Config**: Gemini-3-Flash-Preview:Cloud, tools: `read`, `bash`.
- **Output**: `# Security/Fundamentals Audit: [Topic]` (Markdown).

# SKILLS

Skills provide specialized logic or instructions for specific domains.

**alloy-dev** ([Details](.omp/skills/alloy-dev/SKILL.md))
- **Domain**: WIT evolution, WASM plugin development, and standard verification.
- **Usage**: Invoke for WIT interface changes, kernel-plugin bridge issues, or test failures.
- **Workflow**:
  1. `wit/alloy.wit` updates.
  2. `just build-all` for binding regeneration.
  3. `plugins/wasm/` implementation.


# COMMON PITFALLS & LESSONS LEARNED

**WASM `cabi_realloc` Memory Allocation Mismatches**
- **Symptom**: Panics in tests (e.g., `TestOmniPaletteSearch`) or at runtime when passing structs across the Host-WASM boundary.
- **Cause**: If a field is added to a WIT file (e.g., `Intent` added to a struct) but not fully populated/accounted for in the host runtime's allocation calculation, the host allocates less memory (e.g., 40 bytes) than the guest requires (e.g., 52 bytes), causing memory corruption or panics.
- **Fix**: Whenever updating `wit/alloy.wit` with new fields, verify that `pkg/wasm/runtime/runtime.go` (and related WASM host implementation files) correctly accounts for the new struct sizes during `cabi_realloc`.