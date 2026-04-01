# Phase 8 Planning: Agnostic Contextual Identity

Building on the Phase 7 milestones (**Project Manifests** and **Universal Widgets**), we will implement a namespace-based security model.

## 1. Contextual IAM Updates
The `IAM` system service currently performs a flat check. It needs to support hierarchal namespaces.

- **IAM Enhancement**: Update `IAMManager` in `pkg/kernel/iam.go` to handle capabilities with a `/` prefix.
- **Admin API**: Add a `grant_namespace_capability` method to allow trusted services (like `project`) to attest to a user's role in a specific namespace.

## 2. Discovery Filtering
The `command-manager` should be the first service to use the new IAM logic.

- **Filtered Discovery**: Modify the `discover` method in the Kernel registry to check the caller's identity against each registered capability's requirements.
- **Omni-Palette**: Ensure results returned to the frontend via the `omni-palette` plugin are already filtered by the user's current project context.

## 3. Scoped Event Broadcasting
The `events` and `widget-manager` services must support **Scoped Publication**.

- **Pruned Broadcasting**: When a plugin publishes an event with a `scope` (e.g., `prj-123`), the `EventManager` must only deliver that event to subscribers who possess the `prj-123/*` capability.
- **Privacy**: This ensures that a user who is not a member of a project cannot see the project's widgets, chat, or buffer-update events in their stream.

## 4. Manifest Securty Integration
The `project` WASM plugin will be updated to read a `security` section in the `alloy-project.json`.

- **Attestation Flow**: When a project is opened, the `project` plugin will call the Kernel's `iam:grant_namespace_capability` for each authorized user in the manifest.
- **Zero-Trust**: The Core stays agnostic of projects; the `project` plugin handles the business logic of mapping fingerprints to project roles.

## Success Criteria
- [ ] A "Guest" user can only see a subset of commands in the Omni-Palette.
- [ ] Project-specific widgets only appear for users authorized in the `alloy-project.json`.
- [ ] A user can be an `admin` in one project and a `viewer` in another simultaneously.
- [ ] If no project is active, the system defaults to global (Phase 7) security behavior.

---

**Next Steps**: Implementation of namespaced `Enforce()` logic in `pkg/kernel/iam.go`.
