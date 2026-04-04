# ALLOY: MODULAR TEAM COORDINATION

**1. KERNEL (THE SUBSTRATE)**
Go backend managing Plugins, Messages, Security, and Events. Provides stable infra for coordination.

**2. PROJECT (THE EFFORT)**
Defined in the kernel (or `alloy-project.json`). Specifies:
- Required Plugins & Capabilities
- User Roles (Editor, Planner, Reviewer)

**3. FRONTEND (THE GATEWAY)**
Interaction layer (TUI, GUI, Web). Handles IPC and Hardware.

**4. WORKSPACE (THE SYNTHESIS)**
The frontend's merge of **Project Tools** (shared) and **User Tools** (private) into one interface.

---

## Documentation Index

### Core Concepts
- [Architecture](docs/core/ARCHITECTURE.md) - The coordination kernel design.
- [Coding Guidelines](docs/core/CODING_GUIDELINES.md) - Best practices and standards.
- [Security](docs/core/SECURITY.md) - IAM, mTLS, and isolation details.

### Frontends
- [Frontend Philosophy](docs/frontends/FRONTENDS.md) - Workspace composition engine.
- [TUI Design](docs/frontends/TUI_DESIGN.md) - Terminal interface specifics.
- [Modal Interaction](docs/frontends/MODAL_DESIGN.md) - Intent-based modality.

### Plugins & Development
- [Plugin System](docs/plugins/PLUGINS.md) - Developing WASM plugins.
- [Build & Plugins](docs/plugins/BUILD_AND_PLUGINS.md) - Compilation and loading.
- [Group Chat](docs/plugins/GROUP_CHAT.md) - Real-time communication plugin.

### Planning & Roadmap
- [Roadmap](docs/planning/ROADMAP.md) - Current progress and future phases.
- [Phase 11 Planning](docs/planning/PHASE_11_PLANNING.md) - Active phase details (Lifecycle & Audit).

- [Archives](docs/planning/archives/) - Historical planning documents and lessons learned.

---

## Guidelines for AI Collaborators
- [Agent Rules & Roles](AGENTS.md) - Mandatory workflows and role definitions.
