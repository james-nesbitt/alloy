# Task for crew-worker

# Task Assignment

**Task ID:** task-6
**Task Title:** Hardware-Verified Identity (mTLS)
**PRD:** docs/planning/PHASE_12_PLANNING.md


## Your Mission

Implement this task following the crew-worker protocol:
1. Join the mesh
2. Read task spec to understand requirements
3. Start task and reserve files
4. Implement the feature
5. Commit your changes
6. Release reservations and mark complete

## Concurrent Tasks

These tasks are being worked on by other workers in this wave. Discover their agent names after joining the mesh via `pi_messenger({ action: "list" })`.

- task-3: Intent Delegation & Collaboration
- task-4: Headless Architecture & Zero-Frontend Clients
- task-5: State Reconciliation & Conflict Resolution
- task-7: Role Attestation & Signed Audit Logs

## Recent Activity

20:44 ZenArrow started task-1 — Actor Foundation & IAM Evolution
20:44 DarkQuartz started task-2 — Proactive Interventions (intent:propose)
20:44 ZenArrow ✦ Starting task-1 (Actor Foundation & IAM Evolution) — will update pkg/kernel/iam.go to formalize actor: prefix and define core roles (Developer, Auditor, Reviewer).
20:44 ZenArrow reserved pkg/kernel/iam.go — task-1
20:45 DarkQuartz ✦ Starting task-2 (Proactive Interventions) — will update pkg/kernel/intent_broker.go and pkg/kernel/events.go to support proactive suggests and triggers.

## Task Specification

# Hardware-Verified Identity (mTLS)

Integrate TPM or Secure Enclave support for mTLS key management. Update pkg/security/pki to support hardware-backed key generation and certificate signing. Enhance pkg/ipc/mtls.go to utilize these hardware-stored keys for kernel-to-actor and actor-to-actor communication.


## Plan Context

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

## 4. Paralleli

[Spec truncated - read full spec from .pi/messenger/crew/plan.md]
## Coordination

**Message budget: 10 messages this session.** The system enforces this — sends are rejected after the limit.

**Broadcasts go to the team feed — only the user sees them live.** Other workers see your broadcasts in their initial context only. Use DMs for time-sensitive peer coordination.

### Announce yourself
After joining the mesh and starting your task, announce what you're working on:

```typescript
pi_messenger({ action: "broadcast", message: "Starting <task-id> (<title>) — will create <files>" })
```

### Coordinate with peers
If a concurrent task involves files or interfaces related to yours, send a brief DM. Only message when there's a concrete coordination need — shared files, interfaces, or blocking questions.

```typescript
pi_messenger({ action: "send", to: "<peer-name>", message: "I'm exporting FormatOptions from types.ts — will you need it?" })
```

### Responding to messages
If a peer asks you a direct question, reply briefly. Ignore messages that don't require a response. Do NOT start casual conversations.

### On completion
Announce what you built:

```typescript
pi_messenger({ action: "broadcast", message: "Completed <task-id>: <file> exports <symbols>" })
```

### Reservations
Before editing files, check if another worker has reserved them via `pi_messenger({ action: "list" })`. If a file you need is reserved, message the owner to coordinate. Do NOT edit reserved files without coordinating first.

### Questions about dependencies
If your task depends on a completed task and something about its implementation is unclear, read the code and the task's progress log at `.pi/messenger/crew/tasks/<task-id>.progress.md`. Dependency authors are from previous waves and are no longer in the mesh.

### Claim next task
After completing your assigned task, check if there are ready tasks you can pick up:

```typescript
pi_messenger({ action: "task.ready" })
```

If a task is ready, claim and implement it. If `task.start` fails (another worker claimed it first), check for other ready tasks. Only claim if your current task completed cleanly and quickly.

## Available Skills

Read any skill that matches what you're implementing.

**Recommended for this task:**
  git-workflow — Git workflow assistant for branching, commits, PRs, and conflict resolution. Use when user asks about git strategy, branch management, or PR workflow.
    /var/home/jnesbitt/.pi/agent/skills/git-workflow/SKILL.md

**Also available:**
  claymorphism — Claymorphism design system skill. Use when building soft, puffy, clay-like UI components with large radii, dual inner shadows, and offset outer shadows.
    /var/home/jnesbitt/.pi/agent/skills/claymorphism/SKILL.md
  context7 — Search and query up-to-date documentation for any programming library via Context7 API. Use when you need current docs, code examples, or API references for libraries and frameworks.
    /var/home/jnesbitt/.pi/agent/skills/context7/SKILL.md
  debug-helper — Debug assistant for error analysis, log interpretation, and performance profiling. Use when user encounters errors, crashes, or performance issues.
    /var/home/jnesbitt/.pi/agent/skills/debug-helper/SKILL.md
  glassmorphism — Glassmorphism design system skill. Use when building frosted-glass UI components with blur, transparency, and layered depth effects.
    /var/home/jnesbitt/.pi/agent/skills/glassmorphism/SKILL.md
  liquid-glass — Apple Liquid Glass design system. Use when building UI with translucent, depth-aware glass morphism following Apple's design language. Provides CSS tokens, component patterns, dark/light mode, and animation specs.
    /var/home/jnesbitt/.pi/agent/skills/liquid-glass/SKILL.md
  neubrutalism — Neubrutalism design system skill. Use when building bold UI with thick borders, offset solid shadows, high saturation colors, and minimal border radius.
    /var/home/jnesbitt/.pi/agent/skills/neubrutalism/SKILL.md
  quick-setup — Detect project type and generate .pi/ configuration. Use when setting up pi for a new project or when user asks to initialize pi config.
    /var/home/jnesbitt/.pi/agent/skills/quick-setup/SKILL.md
  web-fetch — Fetch a web page and extract readable text content. Use when user needs to retrieve or read a web page.
    /var/home/jnesbitt/.pi/agent/skills/web-fetch/SKILL.md
  web-search — Web search via DuckDuckGo. Use when the user needs to look up current information online.
    /var/home/jnesbitt/.pi/agent/skills/web-search/SKILL.md

To load a skill: read({ path: "<skill-path>" })
