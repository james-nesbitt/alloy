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
