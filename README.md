# Alloy: The Team-Project Micro-kernel

Alloy is a secure, lightweight, and high-performance micro-kernel built in Go, designed specifically for collaborative development and project-centric workflows. It acts as a secure message bus and orchestration layer for high-performance WASM (WebAssembly) plugins that communicate via the WASM Component Model (WIT).

## The Core Philosophy

Alloy is not just a tool; it is a **Project-First Workspace**. Unlike traditional IDEs or CLI tools that treat the "project" as a secondary folder, Alloy treats the active project as the top-level entity, coordinating shared identity, persistent state, and real-time team collaboration.

### 1. Secure-by-Default (mTLS & IAM)
Every connection to the Alloy kernel is identity-verified using mTLS or Unix credentials. A synchronous **IAM Interceptor** validates every message before it reaches a plugin, ensuring zero-trust security even within the application boundary.

### 2. Project-Centric Context
Alloy bridges the gap between different tools (AI, Git, Chat, Editors) by providing a unified **Project Manifest**. When you open Alloy in a project directory:
- **IAM Policies** are scoped to the project.
- **AI Workers** gain a semantic index of project files and history.
- **Shared Buffers** are synchronized across the team.

### 3. Composable "Front-end Arbitrated" UI
Alloy uses a **Data-Driven UI Protocol**. Plugins do not render pixels; they provide **Summaries**, **Actions**, and **State Payloads**. The Frontend (TUI, GUI, or Web) acts as an arbitrator, deciding how to display this data based on:
- **Project Manifests**: Dictating what information is important for this workspace.
- **User Preference**: Dictating how those components are laid out (Grids, Panes, or Overlays).

### 4. High-Performance Plugin Ecosystem (WASM)
Application logic lives in isolated WASM components. This provides language-agnostic SDKs (Go, Rust, C), hot-reloading, and strict sandboxing.

## 🚀 Quick Start

To build the entire project (Core, Plugins, GUIs, CLI):

```bash
# 1. Install prerequisites (Go 1.25+, TinyGo 0.33+, wit-bindgen-cli, just)
# 2. Build everything
just build-all
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
- [Coding Guidelines](docs/CODING_GUIDELINES.md)

## ⚖️ License

Alloy is released under the MIT License.
