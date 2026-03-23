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
- **`cmd/alloy`**: Tooling for managing backends.
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
## 🚨 POSITIVE CONTROL GUIDELINES (MANDATORY) 🚨

These guidelines are **non-negotiable**. Failure to follow these results in repository corruption and loss of project history integrity.

### 1. ABSOLUTELY NO COMMITS OR MERGES TO `main` WITHOUT EXPLICIT PERMISSION
*   **NEVER** commit directly to the `main` branch.
*   **NEVER** merge a feature branch into `main` automatically.
*   **PROCEDURE**: 
    1.  Work ONLY on `feature/` or `fix/` branches.
    2.  Once work is complete and tests pass, **STOP**.
    3.  Present a summary of changes to the user.
    4.  **ASK**: "Work is complete on branch `feature/xyz`. May I merge this into `main`?"
    5.  **WAIT** for a "Yes", "Merge it", or similar affirmative response before proceeding with the merge.

### 2. Git Flow & Branching
- All changes must be made on a dedicated, descriptively named feature branch (e.g., `feature/pki-implementation`).
- Commit changes incrementally to the feature branch with meaningful commit messages.
- If you find yourself on `main` by mistake, move your changes to a new branch immediately using `git checkout -b` and `git reset --hard origin/main`.

## Interaction Workflow

1.  **Read Before Writing**: Always read existing relevant files horizontally to ensure consistent style and approach.
2.  **Validation (Before Asking to Merge)**:
    - **Format**: Always run `just fmt` (or `go fmt ./...`) before committing changes.
    - **Test**: 
        - Always run `just test` (or `go test ./...`) before proposing a merge.
        - **New and changed tests must be verified to pass.**
        - Ensure a minimum of 85% unit test coverage for new code in `pkg/`.
    - **Diagnostic Efficiency**: Use small, focused test harnesses during active debugging.
3.  **Verify Assumptions**: If you aren't sure where a piece of logic should reside, ask for clarification.
4.  **Iterative Development**: Start with small, verifiable pieces before building larger systems.
