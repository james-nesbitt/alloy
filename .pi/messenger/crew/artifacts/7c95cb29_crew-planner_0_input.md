# Task for crew-planner

Create a task breakdown for implementing this PRD.

## PRD: docs/planning/PHASE_12_PLANNING.md

# Phase 12: Autonomous Substrate

## Status: ACTIVE 🚀 (2026-04-04)

**Goal**: Transition Alloy from a persistent knowledge substrate with semantic memory into an active, agentic coordination environment. This phase makes AI actors first-class citizens with identities, roles, and proactive capabilities.

---

### Workstream 1: Agentic Actor Framework
Make AI actors active participants in the team coordination.

- [ ] **Actor Identity & IAM**:
    - Assign per-actor IAM identities (e.g., `actor:claudine`).
    - Standardize Alloy roles (Developer, Auditor, Reviewer, etc.) adaptable by actors.
    - Implement capability-based permissioning for actors within workspaces.
- [ ] **Proactive Interventions**:
    - Actors monitor kernel event streams for specific patterns.
    - Actors suggest "Intents" back to the user or other actors (e.g., "I noticed a security flaw, should I run an audit?").
    - Implement `intent:propose` protocol.
- [ ] **Intent Delegation**:
    - Humans delegating complex sub-tasks to actors with verified execution chains.
    - Actor-to-actor collaboration protocols.

### Workstream 2: Headless Coordination
Decouple coordination from frontends entirely for automated operations.

- [ ] **Zero-Frontend Clients**:
    - Backend-only clients for purely automated workflow steps (e.g., CI/CD agents, periodic auditors).
    - Lightweight "Headless" mode for frontends.
- [ ] **State Reconciliation**:
    - Ensure headless actors stay in sync with GUI/TUI users during complex multi-step operations.
    - Implement conflict resolution for concurrent human/agent edits in shared buffers.

### Workstream 3: Hardware-Verified Identity (Security)
Strengthen the security foundation for machine-human collaboration.

- [ ] **mTLS Enhancements**:
    - Moving towards hardware-backed keys (TPM, Secure Enclave) for kernel-to-actor communication.
- [ ] **Role Attestation**:
    - Cryptographic proofs for role assignments and actor authenticity.
    - Audit logs containing signed attestations of actor actions.

---

## Technical Considerations

### Multi-Agent Interaction
We need to avoid "event feedback loops" where two agents react to each other's actions infinitely. Implement debounce or "intent locking" mechanisms at the kernel level.

### Syncing Headless Actions
Headless actors should still produce "Visual Intent" (e.g., virtual cursors, highlight regions) so humans can observe their work in real-time, even if the actor has no physical screen.


## Available Skills

Workers can load these skills on demand during task execution. When creating tasks, you may include a `skills` array with relevant skill names to help workers prioritize which to read.

  claymorphism — Claymorphism design system skill. Use when building soft, puffy, clay-like UI components with large radii, dual inner shadows, and offset outer shadows.
  context7 — Search and query up-to-date documentation for any programming library via Context7 API. Use when you need current docs, code examples, or API references for libraries and frameworks.
  debug-helper — Debug assistant for error analysis, log interpretation, and performance profiling. Use when user encounters errors, crashes, or performance issues.
  git-workflow — Git workflow assistant for branching, commits, PRs, and conflict resolution. Use when user asks about git strategy, branch management, or PR workflow.
  glassmorphism — Glassmorphism design system skill. Use when building frosted-glass UI components with blur, transparency, and layered depth effects.
  liquid-glass — Apple Liquid Glass design system. Use when building UI with translucent, depth-aware glass morphism following Apple's design language. Provides CSS tokens, component patterns, dark/light mode, and animation specs.
  neubrutalism — Neubrutalism design system skill. Use when building bold UI with thick borders, offset solid shadows, high saturation colors, and minimal border radius.
  quick-setup — Detect project type and generate .pi/ configuration. Use when setting up pi for a new project or when user asks to initialize pi config.
  web-fetch — Fetch a web page and extract readable text content. Use when user needs to retrieve or read a web page.
  web-search — Web search via DuckDuckGo. Use when the user needs to look up current information online.


You must follow this sequence strictly:
1) Understand the PRD
2) Review relevant code/docs/reference resources
3) Produce sequential implementation steps
4) Produce a parallel task graph

Return output in this exact section order and headings:
## 1. PRD Understanding Summary
## 2. Relevant Code/Docs/Resources Reviewed
## 3. Sequential Implementation Steps
## 4. Parallelized Task Graph

In section 4, include both:
- markdown task breakdown
- a `tasks-json` fenced block with task objects containing title, description, dependsOn, and optionally skills (array of skill names from the Available Skills list that are relevant to the task).