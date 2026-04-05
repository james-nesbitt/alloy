# Phase 13 Implementation: Hyper-Orchestration & Simple Projects

Status: ACTIVE 🚀
Date: 2026-04-05

**Goal**: Transition from autonomous substrates into a multi-agent, project-centric orchestration layer. This phase focuses on simplifying the project lifecycle for both humans and agents, and enabling cross-substrate collaboration.

---

## 1. Simple Project Management (SPI)
Allow the system to handle projects without a rigid `provision.json` manifest.

- [ ] **Manifest-less Projects**:
    - Enable `project:init` to create a working directory and metadata without a full JSON specification.
    - Implement a "default" capability set for ad-hoc projects.
- [ ] **Standardized Project Editing**:
    - Implement `project:edit` and `project:refine` intents.
    - Allow agents to modify project metadata, capability assignments, and role descriptions.
- [ ] **Recovery of "Simple Editing" Work**:
    - Attempt a final reconstruction of the lost `allow-simple-editing` feature if old commits are discovered.
    - Otherwise, re-implement the core logic: a simplified "edit-and-apply" loop for project manifests.

## 2. Dynamic Manifest Evolution (DME)
Move from static boot-time loading to a dynamic, evolving coordination environment.

- [ ] **Capability Injection**:
    - Allow plugins to request or "invite" other plugins into their project scope at runtime.
    - Implement `iam:attest-capability` to provide signed proofs for these dynamic assignments.
- [ ] **Agentic Role Escalation**:
    - Let agents request temporary role upgrades (e.g., "Requesting DEVELOPER role for 10 minutes to fix identified bug").
    - Implement a human-in-the-loop (HITL) approval flow for these escalations in TUI/GUI.

## 3. Cross-Substrate Bridging (CSB)
Connect disconnected Alloy instances for a unified orchestration view.

- [ ] **Remote Substrate Bridge**:
    - Implement a lightweight IPC-over-TLS bridge for connecting headless actors on different machines.
    - Standardize the "Substrate Handshake" protocol for shared project discovery.
- [ ] **Distributed Event Sync**:
    - Use the Librarian's event-sourcing to synchronize remote project states.
    - Resolve conflicts in multi-actor workspace edits using the existing `BufferManager` conflict log.

---

## Technical Constraints & Design Principles

- **Acyclic Coordination**: Bridge chains must not create infinite loops in event propagation.
- **Identity Integrity**: Every bridged message must carry an `Attestation` from the originating substrate.
- **Simple First**: The ad-hoc project flow must require zero configuration to start a basic sync session.

---

## Branch Strategy

- All work will occur in `feat/phase-13-hyper-orchestration` or sub-branches (`feat/p13-simple-projects`, etc.).
- `main` remains the stable integrated base.
