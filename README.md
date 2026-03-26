# Alloy: The Modular Team Coordination Framework

Alloy is a secure, high-performance **Workspace Kernel** designed for multi-user, multi-role collaboration. It acts as a central coordinator that manages **Plugins, Messages, Security, Capabilities, and Events**, providing the platform for a team to cooperate in shared, extensible workspaces.

## The Core: A Robust Coordination Kernel

Alloy's kernel is the stable "substrate" of the workspace. Its primary responsibility is to provide the critical infrastructure required for secure communication and state management.

- **Plugin Orchestration**: Dynamic loading, lifecycle management, and strict sandboxing of WASM-based application logic.
- **Message Routing & Events**: A high-speed, identity-aware event bus for routing messages between plugins, frontends, and users.
- **Security & IAM**: Integrated mTLS identity verification and role-based access control (RBAC) enforced at the kernel level for every interaction.
- **Capability Discovery**: A WIT-based (Wasm Interface Type) registry that allows plugins to expose and consume capabilities across the system.

## The Goal: Runtime Collaboration

While the kernel provides the foundation, Alloy’s value is delivered through **Runtime Plugins**. These plugins expand the core with domain-specific functionality to support the true goal of the system: **Team Cooperation**.

1.  **Project-Specific Archetypes**: A single Alloy instance can host diverse project types simultaneously (Coding, Sales, Ops).
2.  **Role-Based Interfaces**: Different users participate in a project through different lenses (Editor, Planner, Reviewer).
3.  **User-Personalized Composition**: Frontends (TUI, GUI, Web) assemble these capabilities into a personalized workspace.

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
