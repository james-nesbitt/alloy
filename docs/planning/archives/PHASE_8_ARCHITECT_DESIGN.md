# ARCHITECT: Phase 8 - Agnostic Contextual Identity

## 1. Vision
Phase 8 refactors the Alloy **IAM (Identity & Access Management)** from a flat, global user-capability mapping to a **Hierarchical Namespace Model**. 

Crucially, the **Kernel remains agnostic of "Projects" or "Workspaces"**. It provides the primitive for **prefixed capabilities** (`namespace/target:method`) and allows trusted services to attest to ephemeral roles within those namespaces.

---

## 2. Core Mechanism: Hierarchical Capabilities

The capability check in `pkg/kernel/iam.go` will be upgraded to support a **two-level intersection**.

### 2.1 The Capability String
Capabilities now follow a syntax that supports metadata-relative scoping:
`[Namespace]/[Target]:[Method]`

*   **Global**: `buffer:read` (Matches any buffer read)
*   **Scoped**: `prj-123/buffer:read` (Matches buffer:read ONLY if the message context is `prj-123`)

### 2.2 Hierarchical RBAC Resolution
When auditing a message, the IAM Manager performs a tiered check:
1.  **Global Level**: Does the Actor have permission for `target:method`?
2.  **Namespace Level**: If the message `Metadata["context"]` is present (e.g., `prj-123`), does the Actor have permission for `prj-123/target:method`?

---

## 3. Ephemeral Role Attestation

Since the Core does not recognize "Projects", it cannot parse `alloy-project.json` for security rules. Instead, the **Project Plugin** acts as an **Identity Proxy**.

### 3.1 The Grant API (Kernel)
The `iam` system service will provide a new administrative method:
`iam:grant_namespace_role(actor_id, namespace, template_id)`

### 3.2 Sequence of Operations
1.  **Project Plugin**: Parses `alloy-project.json`.
2.  **Project Plugin**: Matches a connected user's fingerprint to a role (e.g., `maintainer`).
3.  **Project Plugin**: Authoritatively notifies the Kernel: *"User A is a `maintainer` in namespace `prj-abc`."*
4.  **Kernel**: Injects the corresponding capability overrides into User A's session cache for the `prj-abc` prefix.

---

## 4. Federated Filtered Discovery

The `command-manager` (Core Service) is the backbone of the **Omni-Palette**. It must now provide **Identity-Relative Discovery**.

### 4.1 Filtered Result Set
When an actor calls `discover`, the Kernel correlates:
1.  All registered capabilities.
2.  The requester's active IAM permissions (Global + Contextual).
3.  **Intersection**: Only capabilities the requester is authorized to invoke are returned.

This ensures that a **Guest** in a project never sees **Admin** commands in their search results, preventing "Access Denied" frustration.

---

## 5. Viewport & Event Guarding (Scoped Distribution)

The **Widget System** (from Phase 7) and the **EventManager** will be upgraded to support **Scoped Publication**.

### 5.1 Redacted Event Stream
When an event is published with a `namespace` label:
`events:publish(topic: "chat:message", scope: "prj-abc")`

The Kernel's event dispatcher will prune the distribution list. Only subscribers who possess the `prj-abc/*` capability (or specific topic capability under that prefix) will receive the message on their IPC socket.

### 5.2 Widget Pruning
The `WidgetManager` will similarly filter `dashboard:widget-registered` broadcasts. If a widget is scoped to `prj-abc`, a developer who is NOT part of that namespace will never see it, and their TUI will not allocate a pane for it.

---

## 6. Implementation Checklist

- [x] **Core IAM**: Refactor `pkg/kernel/iam.go` to handle `/` delimiter in `Enforce()`.
- [x] **Core IAM**: Add `GrantNamespaceCapability` to the IAM internal service.
- [x] **Core Registry**: Update `command-manager` to filter discovery by the caller's identity.
- [x] **Core Events**: Add `scope` metadata handling to publication.
- [x] **Core Projects**: (WASM Plugin) Update to ingest manifest `security` and call the Core IAM APIs.
- [x] **Auth Documentation**: Update `docs/SECURITY.md` to explain hierarchical scoping.
