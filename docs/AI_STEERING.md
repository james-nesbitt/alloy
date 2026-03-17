# AI Steering & Guidelines for Alloy

This document defines how AI agents, like yourselves, should interact with the Alloy project to maintain architectural integrity and consistency.

## Design Philosophy

- **Micro-kernel-centric**: Keep the core small. If functionality can be a WASM plugin, it *should* be a WASM plugin.
- **IPC-first**: 
    - Design interactions assuming they will occur over a message bus. 
    - Leverage the event-driven nature (Pub/Sub) for decoupled communication.
    - Avoid direct imports for core functionality when possible.
- **Golang-centric**: The core, backends, and TUI/GUI frontends are primarily Go.
- **Modular and Extensible**: Each component should be replaceable.

## Project Structure Guidelines

- **`cmd/alloy-core`**: The main backend server.
- **`cmd/alloy-cli`**: Tooling for managing backends.
- **`cmd/alloy-tui`**, `cmd/alloy-gui`, `cmd/alloy-web`: Frontend entry points.
- **`pkg/kernel`**: Core logic for the backend.
- **`pkg/wasm`**: WASM runtime and plugin management logic.
- **`pkg/ipc`**: IPC message definitions and bus implementation.
- **`plugins/`**: Source for WASM plugins.

## Code Standards

- **Coding Guidelines**: Adhere to the project's [Coding Guidelines](./CODING_GUIDELINES.md).
- **Format and Lint**: Ensure all code is formatted (`go fmt`) and passes linting (`golangci-lint`) as defined in the `justfile`.
- **Error Handling**: Use idiomatic Go error handling with a preference for `fmt.Errorf("...: %w", err)`.
- **Extensive Logging**: Ensure every major component and interaction is logged to facilitate debugging.

## Guidelines for AI Agents

- **Prioritize the Architecture**: When suggesting features, consider if they belong in the kernel, a plugin, or a frontend.
- **Maintain Separations**: Ensure that frontends stay frontend-focused, and the kernel remains the orchestrator.
- **WASM Constraints**: 
    - Remember that WASM plugins have restricted access. 
    - Use the kernel's message bus or RefID system for resource access (files, network).
    - Respect "Fuel" and memory limits in plugin logic.
    - Implement non-blocking host calls.
- **Security by Design**: Always think about multi-user permissions and sandbox isolation. (Note: mTLS-based identity is currently deferred).
- **XDG Compliance**: Always respect XDG environment variables (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_RUNTIME_DIR`, etc.). 
    - Default to `~/.config/alloy`, `~/.local/share/alloy`, and `~/.cache/alloy`.
    - Volatile files (sockets, PIDs) should reside in `XDG_RUNTIME_DIR/alloy`.
    - Paths should only fall back to the current directory (`pwd`) when explicitly requested or during localized development.
- **Audit Everything**: Ensure new features or communication paths are integrated into the audit logging system.
- **Follow the Path**: When adding new files, place them in the established directory hierarchy.

### Interaction Workflow

1.  **Read Before Writing**: Always read existing relevant files horizontally to ensure consistent style and approach.
2.  **Git Flow**:
    - Start all new feature work on a dedicated git branch (e.g., `feature/pki-implementation`).
    - Commit changes incrementally to the branch.
    - Branches must be merged to `main` only after approval.
3.  **Validation**:
    - **Format**: Always run `just fmt` (or `go fmt ./...`) before committing changes to ensure code consistency.
    - **Test**: 
        - Always run `just test` (or `go test ./...`) before committing.
        - **New and changed tests must be verified to pass.**
        - Ensure a minimum of 85% unit test coverage for new code in `pkg/`.
4.  **Verify Assumptions**: If you aren't sure where a piece of logic should reside, ask for clarification.
5.  **Iterative Development**: Start with small, verifiable pieces (e.g., the IPC message definitions) before building larger systems.
