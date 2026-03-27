# AGENT_GUIDELINES: MANDATORY WORKFLOW

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
- *Note*: Merges are only permitted if VERIFICATION passes or explicit User "Autonomy" is granted.

**3. ROLE-BASED ACCESS CONTROL**
- **Documentation Only**: PLANNER and ARCHITECT roles are restricted to `docs/`, `plans/`, and `roadmap` updates. They must NEVER modify code or run tests.
- **Code Modification**: Only DEVELOPER and REVIEWER roles may modify the codebase (`.go`, `.wit`, `.js`).

**4. TESTING PROTOCOL**
- **DEVELOPER**: Must write unit tests alongside new code but is NOT required to run the full `just test-all` suite.
- **REVIEWER**: Responsible for the full integration verification.

# AGENT ROLES (INVOKE VIA SUBAGENT)

```json
subagent({ agent: "[role]", task: "[your instruction]" })
```

**PLANNER (Feature Planner)**
- *Task*: Define feature requirements, WIT interfaces, and project manifest impact.
- *Limit*: Documentation only. No code edits. **NO TESTING.**
- *Output*: `# Implementation Plan` (Markdown).

**ARCHITECT (Refactor Planner)**
- *Task*: Analyze code debt, propose structural refactors.
- *Limit*: Documentation only. No code edits. **NO TESTING.**
- *Output*: `# Architectural Design` (Markdown).

**DEVELOPER (Implementer)**
- *Task*: Execute a confirmed PLANNER/ARCHITECT plan.
- *Requirement*: Always work in a `feat/` or `fix/` branch.
- *Task*: Write tests for new logic. Spot test as needed.
- *Limit*: Do not run full test suites. **NO MERGING.**

**REVIEWER (Quality/Merge Guard)**
- *Task*: Critique code. Perform FULL VERIFICATION (`just test-all`).
- *Authority*: Only role authorized to merge into `main`.

**AUDITOR (Security/Fundamentals)**
- *Task*: Evaluate for security holes, leaks, or bad Go patterns.
- *Limit*: Reporting only. No edits. No testing.
