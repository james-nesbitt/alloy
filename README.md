# Alloy: The Modular Team Coordination Framework

Alloy is a secure, high-performance coordination framework designed for multi-user, multi-role collaboration. It acts as a **Modular Workspace Engine** that composes backend capabilities (via WASM plugins) into specialized interfaces tailored to specific project types and user roles.

## The Philosophy: Unified Core, Specialized Workspaces

Alloy is more than a development tool; it is a platform for **Team Orchestration**. While the backend provides a stable foundation for identity, state, and eventing, the frontends (TUI, GUI, Web) are responsible for assembling these capabilities into a coherent environment that matches the user's current goal.

### 1. Project-Specific Archetypes
A single Alloy instance can host diverse project types simultaneously. The layout, available tools, and data-views change based on the **Project Archetype**:
- **Coding**: Focused on buffers, Git integration, and AI-assisted refactoring.
- **Sales/CRM**: Prioritizes lead tracking, communication history, and shared notes.
- **Operations**: Centers on real-time log streams, health metrics, and task queues.
- **Support**: Combines chat, ticket management, and knowledge-base search.

### 2. Role-Based Interfaces
Different users participate in a project through different lenses. Alloy's frontend dynamically adjusts the UI based on the **User Role**:
- **The Editor**: Full-screen focus on creation, high-performance modal interaction, and active feedback loops.
- **The Planner**: Overview-heavy layout with task boards, timelines, and resource allocation widgets.
- **The Reviewer**: Comparative views, annotation tools, and audit logs.
- **The Support Agent**: Multi-channel communication streams and quick-access documentation cards.

### 3. User-Personalized Composition
Alloy empowers users to define their own **Workspace Composition**. You can mix project-wide components (like a shared team chat) with user-specific content (like a private scratchpad or a customized dashboard) to create the environment that best supports your workflow.

## 🚀 Quick Start

To build the entire project (Core, Plugins, GUIs, CLI):

```bash
# 1. Install prerequisites (Go 1.25+, TinyGo 0.33+, wit-bindgen-cli, just)
# 2. Build everything
just build-all

# 3. (Optional) Install web dependencies for testing
just install-web-deps
```

The primary entry point is the `alloy` tool:

```bash
# Start a standalone core
./build/bin/alloy core --listen unix://./alloy.sock

# Start TUI with a dedicated core instance
./build/bin/alloy tui --dedicated

# Connect TUI to an existing backend
./build/bin/alloy tui --socket unix://./alloy.sock
```

The resulting binaries will be placed in the `./build/` directory:
- `./build/bin/alloy`: Unified CLI entry point
- `./build/bin/alloy-core`: Main backend kernel
- `./build/bin/alloy-tui`: Terminal user interface
- `./build/bin/alloy-gui`: Native graphical user interface
- `./build/wasm/*.wasm`: Compiled application plugins

## 🛠️ Build and Development

Alloy uses a `justfile` for orchestration. Key commands include:

| Command | Description |
|---------|-------------|
| `just build-all` | Build both the Core and all Plugins |
| `just build-core` | Build the Go backend binary |
| `just build-plugins` | Build all WASM-based plugins |
| `just setup-dev` | Configure Go workspace and replacements |
| `just test` | Run the complete test suite |

For more detailed information, see [Build and Plugin Guidelines](docs/BUILD_AND_PLUGINS.md).

## 📁 Project Structure

- `cmd/`: Binaries for the backend, CLI, and frontends.
- `pkg/`: Core libraries, kernel, and **WIT-based WASM runtime**.
- `plugins/wasm/`: Specialized application logic as WASM plugins.
- `api/`: Shared IPC message definitions and API types.
- `wit/`: WebAssembly Interface Type (WIT) definitions.
- `docs/`: In-depth documentation.

## 📖 Key Documentation

- [Architecture Overview](docs/ARCHITECTURE.md)
- [Build and Plugin Guidelines](docs/BUILD_AND_PLUGINS.md)
- [Frontend Details](docs/FRONTENDS.md)
- [Security Framework](docs/SECURITY.md)
- [Implementation Roadmap](docs/ROADMAP.md)
- [Modal Interaction Design](docs/MODAL_DESIGN.md)
- [Coding Guidelines](docs/CODING_GUIDELINES.md)

## ⚖️ License

Alloy is released under the MIT License.
