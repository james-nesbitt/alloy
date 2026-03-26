# AGENT_GUIDELINES: MANDATORY WORKFLOW

**1. BRANCHING**
- NEVER work on or commit to `main`.
- USE `feat/`, `fix/`, or `docs/` branches.

**2. VERIFICATION**
Before any merge, MUST run and pass:
- `just fmt`
- `just build-all`
- `just test`

**3. MERGE PROTOCOL**
Merges to `main` ONLY if:
- A: User gives explicit "Yes"/"Merge" after a task summary.
- B: ALL tests passed IN SESSION and User pre-authorized "Autonomy".

**4. CONTEXT CHECK**
Before edits, MUST read:
- `README.md` (Concepts)
- `docs/ROADMAP.md` (Phase logic)
- `wit/alloy.wit` (Interfaces)

# AGENT ROLES (INVOKE BY NAME)

**ARCHITECT (Refactor Planner)**
- *Task*: Analyze code debt, propose structural refactors.
- *Rule*: Focus on decoupling, interface stability, and DRY. NO Implementation.

**PLANNER (Feature Planner)**
- *Task*: Define feature requirements, WIT interfaces, and project manifest impact.
- *Rule*: Match `docs/ROADMAP.md` goals. Output: Detailed implementation plan.

**DEVELOPER (Implementer)**
- *Task*: Execute a confirmed PLANNER/ARCHITECT plan.
- *Rule*: Follow `wit/` bindings exactly. Must run VERIFICATION (Rule 2).

**REVIEWER (Quality/Merge Guard)**
- *Task*: Critique code on a `feat/` or `fix/` branch before merge.
- *Rule*: Responsible for ENFORCING test success and coverage before any merge. Verify logic, interface stability, and documentation parity.

**AUDITOR (Security/Fundamentals)**
- *Task*: Evaluate current state for security holes, performance leaks, or bad Go patterns.
- *Rule*: Focus on IAM bypass, memory safety, and concurrency bugs.
