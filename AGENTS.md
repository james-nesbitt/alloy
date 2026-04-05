# AGENT MANDATORY OPERATING PROCEDURES (AMOP)

## 🚨 THE SUPREME DIRECTIVE: ISOLATION 🚨

To prevent data loss, branch contamination, and build instability, all agents **MUST** follow this isolation protocol. Failure to follow this protocol is a critical system failure.

### 1. STARTING AN EFFORT
Every time a new plan or task is initiated:
- **NEW BRANCH**: You **MUST** create a new feature (`feat/`), fix (`fix/`), or documentation (`docs/`) branch.
- **NEW WORKTREE**: You **MUST** create a new git worktree for this branch. 
  - `git worktree add ../alloy-<branch-name> <branch-name>`
- **NEVER** work directly in `main`.
- **NEVER** work in an existing worktree used by another task.

### 2. EXECUTION PHASE
- All development, documentation, and experimentation **MUST** happen within the isolated worktree.
- If you find yourself in `main`, **STOP IMMEDIATELY**. Do not edit. Do not commit. Move to a branch and worktree.

### 3. VERIFICATION (MANDATORY)
When the effort is considered "complete," you **MUST** run the full verification suite within the worktree before proposing a merge:
1. **LINTING**: `just fmt` (Must pass without manual intervention).
2. **FULL BUILD**: `just build-all` (All 14+ plugins and all frontends must compile).
3. **FULL TEST**: `go test ./pkg/...` and any relevant plugin tests.
4. **NO SUPPRESSION**: You **MUST NOT** ignore failures or suppress tests to "finish" a task.

### 4. FINALIZATION & CLEANUP
Closing an effort involves strict surgical cleanup:
- **MERGE**: Merge the verified branch into `main`.
- **SURGICAL REMOVAL**: Remove the git worktree and the local branch associated with the COMPLETED effort.
- **🚨 STOP 🚨**: 
  - **DO NOT** merge other pending branches.
  - **DO NOT** remove other active worktrees.
  - **DO NOT** perform "bulk" cleanup of the `../` directory.

---

## 🛠 AGENT ROLES & CONSTRAINTS

### PLANNER / ARCHITECT
- **Permission**: READ codebase. WRITE to `docs/` and `plans/`.
- **Mandate**: Define high-level requirements and structural changes.

### DEVELOPER
- **Permission**: WRITE to specific subsystem files as defined in the plan.
- **Mandate**: Execute the plan with 100% fidelity. Write unit tests for all new logic.
- **Constraint**: Prohibited from merging to `main`.

### REVIEWER
- **Permission**: SOLE authority to merge to `main`.
- **Mandate**: Final gatekeeper for quality and the **AMOP Verification Suite**.

---

## ⚠️ LESSONS LEARNED (DO NOT REPEAT)

### ❌ The "Lost Worktree" Incident
- **Failure**: An agent performed a bulk cleanup or failed to track which worktree belonged to which effort, leading to the loss of the `allow-simple-editing` logic.
- **Correction**: Worktrees must be named descriptively and removed **ONLY** after their specific branch is merged and verified.

### ❌ The "Main Branch Contamination"
- **Failure**: Agents editing directly in `main` leading to broken builds for other agents.
- **Correction**: The `main` branch is a read-only source of truth for agents. Implementation happens elsewhere.

### ❌ API Signature Mismatches
- **Failure**: Changing a core interface (e.g., `frontend.Client.RouteMessage`) without updating all callers.
- **Correction**: `just build-all` is non-negotiable. If it doesn't build, it doesn't exist.

---

## 🧬 SKILLS & TOOLS

- **alloy-dev**: Specialized skill for WIT evolution and plugin development.
- **lsp**: Use `lsp references` before changing any exported symbol.
- **ast_grep/ast_edit**: Use for structural changes instead of regex hacks.
