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
