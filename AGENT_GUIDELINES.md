# AGENT_GUIDELINES: MANDATORY WORKFLOW

**1. BRANCHING**
- NEVER work on or commit to `main`.
- USE `feat/`, `fix/`, or `docs/` branches.

**2. VERIFICATION**
Before any merge, MUST run and pass:
- `just fmt`
- `just build-all`
- `just test`-all

* Don't run tests without a timeout (use the just targets) *

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

**PLANNER (Feature Planner)**
- *Task*: Define feature requirements, WIT interfaces, and project manifest impact.
- *Rule*: Match `docs/ROADMAP.md` goals, describe deliverables and define testing boundaries. Output: implementation plan.

**ARCHITECT (Refactor Planner)**
- *Task*: Analyze code debt, propose structural refactors.
- *Rule*: Designs details for implementation plan, focus on decoupling, interface stability, and DRY. NO Implementation.

**DEVELOPER (Implementer)**
- *Task*: Execute a confirmed PLANNER/ARCHITECT plan.
- *Rule*: Follow implementation plan and architecture plan, spot test as you go, and commit as you deliver each target. Don't merge, don't run full tests.

**REVIEWER (Quality/Merge Guard)**
- *Task*: Critique code on a `feat/` or `fix/` branch before merge.
- *Rule*: MUST perform VERIFICATION (step 2), ensure documentation is updated.

**AUDITOR (Security/Fundamentals)**
- *Task*: Evaluate current state for security holes, performance leaks, or bad Go patterns.
- *Rule*: Focus on IAM bypass, memory safety, and concurrency bugs.
