# Alloy

Alloy is a multi-user, micro-kernel styled application platform written in Go. It features a modular backend kernel and relies heavily on WASM plugins for functionality.

## Core Concepts

- **Micro-kernel Architecture**: The core backend is minimal, handling message routing, plugin lifecycle, and security.
- **WASM Plugins**: Core functionality is extended through WebAssembly (WASM) plugins.
- **Frontend Agnostic**: A single backend can support multiple simultaneous frontend connections via IPC (Unix sockets or network).
- **Multi-user**: Designed to handle multiple users and their respective permissions.

## Frontends

Alloy supports multiple frontend implementations:
- **TUI**: A terminal-based user interface.
- **GUI**: A Wayland-compatible graphical user interface.
- **Web**: A web-based interface (Go backend with JS frontend).

## Documentation

For more information, see the following documents:
- [Architecture Overview](docs/ARCHITECTURE.md)
- [Frontend Details](docs/FRONTENDS.md)
- [Security Framework](docs/SECURITY.md)
- [Implementation Roadmap](docs/ROADMAP.md)
- [Plugin Details](docs/PLUGINS_ROADMAP.md)
- [Coding Guidelines](docs/CODING_GUIDELINES.md)
- [AI Steering & Guidelines](docs/AI_STEERING.md)

## Project Structure

- `cmd/`: Binaries for the backend, CLI, and frontends.
- `pkg/`: Core libraries and logic.
- `plugins/`: WASM plugin implementations and SDK.
- `docs/`: Project documentation.
- `api/`: API and IPC message definitions.
