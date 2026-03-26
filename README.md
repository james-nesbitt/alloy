# Alloy: The Modular Team Coordination Framework

Alloy is a secure, high-performance **Workspace Kernel** designed for multi-user, multi-role collaboration. It acts as a central coordinator that manages **Plugins, Messages, Security, Capabilities, and Events**, providing the foundation for how teams cooperate.

## Core Concepts

To understand how Alloy functions, we define three primary layers:

### 1. The Project (Team-Wide)
A **Project** defines the shared functionality and infrastructure for a team working on a single effort. It is the "Source of Truth" for collaboration, specifying:
- **Shared Plugins**: The specific capability providers (e.g., Coding, Sales, or Ops tools) required for the effort.
- **Shared State**: The data-streams, buffers, and knowledge graphs the team interacts with.
- **Security & Roles**: The policies and identity mappings that define who can do what (Editors, Planners, Reviewers).

### 2. The Frontend (Interaction)
A **Frontend** is how an individual user interacts with the backend kernel. Frontends provide the hardware-specific bridge between the user and the project data.
- **TUI**: High-density terminal interface (Bubbletea).
- **GUI**: Hardware-accelerated native interface (Gio).
- **Web**: Remote access via a modern browser.

### 3. The Workspace (User-Specific)
A **Workspace** is the frontend's collection of **Project Tools** and **User Personal Tools** into a single, cohesive interface. It is the user's specific "Lens" into the project, merging:
- **Project Tools**: Shared components defined by the team (e.g., Team Chat, Project Dashboard).
- **User Tools**: Personal components and "side-cars" (e.g., Private Scratchpads, Personal TODOs).
- **Personalization**: The user's preferred layout, theme, and navigation driver (Vim, Helix, or Meow).

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
