# Alloy: The Hybrid Kernel Workspace

Alloy is a secure, lightweight, and high-performance hybrid kernel built in Go, designed specifically for collaborative development and project-centric workflows. It acts as a secure message bus and orchestration layer for high-performance WASM (WebAssembly) plugins while providing integrated core services for identity, state, and eventing.

## The Core Philosophy: Pragmatic Hybrid Kernel

Alloy has evolved from a pure micro-kernel to a **Pragmatic Hybrid Kernel**. While application logic remains isolated in WASM plugins, the critical "Operating System" services—**IAM, KV, Events, and Telemetry**—are integrated directly into the Go-based kernel. 

This hybrid approach ensures:
- **Zero-Latency Core**: Internal system calls for identity and state management happen at native Go speeds with no serialization overhead.
- **Guaranteed Integrity**: Core security (IAM) and communication (Events) are available immediately upon boot, eliminating bootstrapping deadlocks.
- **Plugin Flexibility**: Application-level logic (AI Agents, Chat, Project Management) continues to benefit from the language-agnostic isolation of the WASM Component Model (WIT).

### 1. Secure-by-Default (Integrated IAM)
Every connection to the Alloy kernel is identity-verified. A built-in **IAM (Identity & Access Management)** system validates every message as it traverses the kernel, ensuring zero-trust security is baked into the "hardware" of the workspace.

### 2. Project-Centric Context
Alloy bridges the gap between different tools (AI, Git, Chat, Editors) by providing a unified **Project Manifest**. When you open Alloy in a project directory:
- **IAM Policies** are scoped to the project and enforced by the kernel.
- **AI Workers** gain a semantic index of project files and history via integrated KV stores.
- **Shared Buffers** are synchronized across the team using the core Event bus.

### 3. Composable "Front-end Arbitrated" UI
Alloy uses a **Data-Driven UI Protocol**. Plugins do not render pixels; they provide **Summaries**, **Actions**, and **State Payloads**. The Frontend (TUI, GUI, or Web) acts as an arbitrator, deciding how to display this data based on project context and user preference.

### 4. High-Performance Plugin Ecosystem (WASM)
While the core is integrated, application logic lives in isolated WASM components. This provides language-agnostic SDKs (Go, Rust, C), hot-reloading, and strict sandboxing.

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
- [Coding Guidelines](docs/CODING_GUIDELINES.md)

## ⚖️ License

Alloy is released under the MIT License.
