# Planning Outline

## 1. PRD Understanding Summary
Phase 12, "Autonomous Substrate," focuses on transforming Alloy from a passive knowledge store into an active, agentic environment. This involves making AI actors first-class citizens with confirmed identities, standardized roles, and the ability to proactively suggest and execute tasks. The phase also addresses "headless" collaboration where backend agents work alongside humans in shared buffers, and strengthens the security layer through hardware-verified identities (TPM/Secure Enclave) and signed attestations.

## 2. Relevant Code/Docs/Resources Reviewed
- **IAM & Identity**: `pkg/kernel/iam.go` currently handles RBAC and namespaces, but needs formalization for actor roles.
- **Routing & Intent**: `pkg/kernel/intent_broker.go` handles goal-based routing, which is the foundation for `intent:propose`.
- **Event Mesh**: `pkg/kernel/events.go` provides the pub/sub event stream that actors will monitor.
- **Security & PKI**: `pkg/security/pki/pki.go` and `pkg/ipc/mtls.go` manage the current software-based mTLS certs.
- **Kernel & Frontends**: `pkg/kernel/kernel.go` manages frontend channels; "headless" mode will require extending this management.

## 3. Sequential Implementation Steps
1. **IAM Extension**: Implementing `actor:` identity prefixes and the tripartite role system (Developer, Auditor, Reviewer).
2. **Proactive Protocol**: Updating `IntentBroker` and `EventManager` to support autonomous suggestions and pattern monitoring.
3. **Delegation Framework**: Handling `intent:delegate` to allow human-to-agent task handoff.
4. **Headless Infrastructure**: Modifying the kernel to support peers that lack a traditional UI but interact with shared state.
5. **Conflict & Sync**: Enhancing the `BufferManager` with reconciliation logic for concurrent human/agent edits.
6. **Hardware Security**: Integrating TPM support for mTLS and log signing.
7. **Verification Layer**: Finalizing role attestations and immutable audit chains.

## 4. Parallelized Task Graph
### Task 1: Actor Foundation & IAM Evolution
Formalize `actor:` identity prefix in IAM. Define and implement core Alloy roles: Developer, Auditor, Reviewer. Update `IdentityManager` to enforce capability-based permissioning for these actor roles within workspaces. Create policy templates for standard agentic behaviors in `pkg/kernel/iam.go`.

**Dependencies**: none

### Task 2: Proactive Interventions (`intent:propose`)
Implement the `intent:propose` and `intent:suggest` protocols in the `IntentBroker`. Add support in `EventManager` for actors to register pattern-based triggers on event streams. Enable actors to send proactive suggestions back to users or other actors with a clear UI-agnostic format.

**Dependencies**: Actor Foundation & IAM Evolution

### Task 3: Intent Delegation & Collaboration
Implement `intent:delegate` in `IntentBroker` to allow humans to assign multi-step tasks to actors. Build the verification chain logic to track sub-task execution. Define actor-to-actor collaboration protocols, allowing one agent to request capabilities from another via the kernel.

**Dependencies**: Actor Foundation & IAM Evolution

### Task 4: Headless Architecture & Zero-Frontend Clients
Develop host-side infrastructure to support backend-only actors that lack a visual frontend channel. Implement a "Headless" mode for the kernel and frontends that allows automated agents (CI/CD, periodic auditors) to participate as full citizens. Ensure proper lifecycle management for these persistent but invisible actors.

**Dependencies**: none

### Task 5: State Reconciliation & Conflict Resolution
Implement state reconciliation logic to keep headless agents in sync with human-driven workspaces. Develop buffer-level conflict resolution for concurrent human/actor edits in `pkg/kernel/buffer_manager.go`. Implement "Visual Intent" broadcasting so headless actors can transmit virtual cursors or highlights to human-facing frontends.

**Dependencies**: Headless Architecture & Zero-Frontend Clients

### Task 6: Hardware-Verified Identity (mTLS)
Transition towards hardware-backed keys (TPM, Secure Enclave) for kernel-to-actor communication. Update `pkg/security/pki` to support hardware-backed key generation and certificate signing. Enhance `pkg/ipc/mtls.go` to utilize these hardware-stored keys for kernel-to-actor and actor-to-actor communication.

**Dependencies**: none

### Task 7: Role Attestation & Signed Audit Logs
Implement cryptographic role attestation to prove an actor's authenticity and authorized role. Update the `LoggerManager` to produce signed audit logs, ensuring every actor-performed action is immutable and verifiable. Use hardware-backed signatures established in the mTLS task.

**Dependencies**: Hardware-Verified Identity (mTLS), Actor Foundation & IAM Evolution

```tasks-json
[
  {
    "title": "Actor Foundation & IAM Evolution",
    "description": "Formalize 'actor:' identity prefix in IAM. Define and implement core Alloy roles: Developer, Auditor, Reviewer. Update IdentityManager to enforce capability-based permissioning for these actor roles within workspaces. Create policy templates for standard agentic behaviors in pkg/kernel/iam.go.",
    "dependsOn": [],
    "skills": ["git-workflow"]
  },
  {
    "title": "Proactive Interventions (intent:propose)",
    "description": "Implement the 'intent:propose' and 'intent:suggest' protocols in the IntentBroker. Add support in EventManager for actors to register pattern-based triggers on event streams. Enable actors to send proactive suggestions back to users or other actors with a clear UI-agnostic format.",
    "dependsOn": ["Actor Foundation & IAM Evolution"],
    "skills": ["git-workflow"]
  },
  {
    "title": "Intent Delegation & Collaboration",
    "description": "Implement 'intent:delegate' in IntentBroker to allow humans to assign multi-step tasks to actors. Build the verification chain logic to track sub-task execution. Define actor-to-actor collaboration protocols, allowing one agent to request capabilities from another via the kernel.",
    "dependsOn": ["Actor Foundation & IAM Evolution"],
    "skills": ["git-workflow", "debug-helper"]
  },
  {
    "title": "Headless Architecture & Zero-Frontend Clients",
    "description": "Develop host-side infrastructure to support backend-only actors that lack a visual frontend channel. Implement a 'Headless' mode for the kernel and frontends that allows automated agents (CI/CD, periodic auditors) to participate as full citizens. Ensure proper lifecycle management for these persistent but invisible actors.",
    "dependsOn": [],
    "skills": ["git-workflow"]
  },
  {
    "title": "State Reconciliation & Conflict Resolution",
    "description": "Implement state reconciliation logic to keep headless agents in sync with human-driven workspaces. Develop buffer-level conflict resolution for concurrent human/actor edits in pkg/kernel/buffer_manager.go. Implement 'Visual Intent' broadcasting so headless actors can transmit virtual cursors or highlights to human-facing frontends.",
    "dependsOn": ["Headless Architecture & Zero-Frontend Clients"],
    "skills": ["git-workflow", "debug-helper"]
  },
  {
    "title": "Hardware-Verified Identity (mTLS)",
    "description": "Integrate TPM or Secure Enclave support for mTLS key management. Update pkg/security/pki to support hardware-backed key generation and certificate signing. Enhance pkg/ipc/mtls.go to utilize these hardware-stored keys for kernel-to-actor and actor-to-actor communication.",
    "dependsOn": [],
    "skills": ["git-workflow"]
  },
  {
    "title": "Role Attestation & Signed Audit Logs",
    "description": "Implement cryptographic role attestation to prove an actor's authenticity and authorized role. Update the LoggerManager to produce signed audit logs, ensuring every actor-performed action is immutable and verifiable. Use hardware-backed signatures established in the mTLS task.",
    "dependsOn": ["Hardware-Verified Identity (mTLS)", "Actor Foundation & IAM Evolution"],
    "skills": ["git-workflow"]
  }
]
```
