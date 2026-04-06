# Alloy Architecture: The Coordination Kernel

Alloy is architected as a **Modular Team Coordination Framework**. The **Alloy Kernel** acts as a central coordinator for **Plugins, Messages, Security, Capabilities, and Events**, enabling shared efforts between diverse users.

---

## 1. Core Architecture Layers (Base Orchestration)

Alloy is an environment-agnostic **Base Host**. It provides a common substrate for multiple independent **Bases** (logical workspaces or projects).

### 1.1 The Base (The Logical Workspace)
A **Base** is a synthesis of project-level tools and personal context into a cohesive instance.
- **Isolation**: Each Base is identified by a unique `BaseID` (e.g., `project-alpha`).
- **Base-Scoping**: The Kernel enforces strict isolation; messages originating from one Base cannot access the KV store, Buffers, or Plugins of another Base unless explicitly authorized.
- **Ephemerality**: Bases can be activated on-demand (e.g., when opening a directory) and deactivated when the user disconnects.

### 1.2 Discovery Providers (Agnostic Sourcing)
Instead of the Kernel having hardcoded "Project Scanning" logic, it delegates discovery to WASM plugins.
- **Capabilities**: Plugins advertise discovery traits (e.g., `base:provider:path` or `base:provider:ipc`).
- **Activation**: When a user opens a resource (like a local folder), the Kernel matches the request to a Provider plugin, maps the resource into the plugin's guest space (WASI), and requests a `BaseManifest`.

### 1.3 The Frontend (The Gateway)
A **Frontend** is the translation layer between the user and the kernel. Each frontend (TUI, GUI, Web) is responsible for:
- **IPC Protocol**: Communicating with the kernel via a secure (mTLS or PeerCreds) socket using the `api.Message` protocol.
- **Hardware Abstraction**: Mapping data-streams from the kernel to the user's terminal, graphics card, or browser engine.
- **Input Handling**: Managing raw keyboard/mouse events and passing them to the active **Modal Driver** (Neovim, Helix, or Meow).

While the kernel provides the infrastructure, the true goals of the application—**Team Cooperation**—are delivered through **Runtime Plugins**. These plugins expand the core by adding domain-specific functionality.

## 2. Platform Standard Services

The Go kernel is responsible for the core lifecycle and the "Operating System" layer of the workspace. Its responsibilities are split between its internal Go-native logic and a tiered ecosystem of services.

#### Tier 1: Integrated Core Services (Go-Native)
- **BaseManager**: Manages the activation lifecycle of independent Bases and matches discovery capabilities to WASM providers.
- **Native Security Router**: A low-latency authorization layer inside the kernel (`pkg/kernel/iam.go`) that protects core message routing using mTLS-verified identities and **Mandatory Base-Scoping**.
- **Buffer Manager**: Integrated shared memory management (`pkg/kernel/buffer_manager.go`) with Base-isolated namespaces.
- **Key-Value Store (KV)**: A high-performance, persistent state store integrated directly into the kernel to eliminate RPC overhead for frequent metadata reads/writes.
- **Event Bus (Pub/Sub)**: The internal central nervous system for message routing and asynchronous event notification.
- **Telemetry & Logging**: Native monitoring of all kernel and plugin interactions.

#### Tier 2: Standard Library Plugins (WASM)
While functionally core to the platform, these are implemented as WASM plugins for extensibility using the [Alloy Guest SDK](../pkg/wasm/guest/sdk.go):
- **IAM Service**: Advanced, granular RBAC (Role-Based Access Control) with resource-level permissions and active security auditing.
- **Knowledge Graph (Indexer)**: A unified activity indexer that listens to cross-plugin event streams (Chat, Buffers, Tasks, Projects) and maintains a persistent, searchable semantic graph.
- **Omni-Palette**: Universal search and command entry point that aggregates results from all registered plugins and provides contextual command boosting.
- **Storage Service**: Virtual Filesystem (WASI) provider and metadata management.
- **Command Manager**: Central registry for all system capabilities and keyboard shortcuts.

#### Host Functions:
- **Message Bus (IPC)**: A high-performance message routing system for cross-plugin and frontend-to-backend communication.
- **WASM Plugin Engine**: A `wazero`-based runtime for loading and executing WASM-based components using the WASM Component Model (WIT).
- **Hot-Reloading**: Enables the `WasmManager` to swap plugin binaries without service interruption.

### 1.2 Communication Paths
Alloy features two distinct communication models:
1. **Direct Go Calls (Internal)**: Internal kernel components communicate via direct method calls. This ensures high throughput and zero-latency for the "Zero-Latency Core."
2. **WIT-based Async Messaging (Plugins/Frontends)**: All 3rd-party code and frontends interact via the Message Bus. The kernel converts internal Go results into the asynchronous, WIT-compatible response format.

---

## 2. Platform Standards & Compliance

- **XDG Compliance**: Respect platform standards for file locations. 
    - Configuration: `~/.config/alloy` (or `XDG_CONFIG_HOME`).
    - Data: `~/.local/share/alloy` (or `XDG_DATA_HOME`).
    - Cache: `~/.cache/alloy` (or `XDG_CACHE_HOME`).
    - Runtime: `XDG_RUNTIME_DIR/alloy` (for volatile files like sockets and PIDs).
- **Audit Everything**: Ensure new features or communication paths are integrated into the audit logging system.

---

## 3. Application Logic (WASM Plugins)

While infrastructure is integrated, all **application-level logic** is offloaded to isolated WASM plugins. 

- **Isolation**: Each plugin runs in a `wazero` sandbox.
- **Hot-Reloading**: The `WasmManager` can swap a plugin binary at runtime while preserving the registry and subscriber state.
- **Standard WIT Surface**: Every plugin communicates with the integrated Core Services via standard WebAssembly Interface Type (WIT) bindings. 
- **Discovery**: The kernel can automatically discover WASM plugins in directories specified via the `--wasm-plugins` flag.
- **Resource Constraints**: Each plugin is assigned a sandbox profile defining limits on memory, CPU "fuel," and internal storage access.

---

## 4. Frontends

Frontends provide user interaction and connect to the backend core via IPC. See [Alloy Frontends](FRONTENDS.md) for more details.
- **Connection Model**: Frontends can connect to an already-running backend or initialize a new one.
- **Multi-Client**: Multiple frontends can connect to a single backend simultaneously.
- **Bootstrapping Process**:
    - **Self-Sourced Configuration**: Each frontend is responsible for loading its own configuration.
    - **Backend Orchestration**: When a frontend starts a new backend, it is responsible for passing the initial backend configuration, including the list of plugins to load and security policy restrictions.
    - **Policy Integration**: Backend and plugin policy restrictions are defined within the configuration and enforced by the kernel upon startup.
- **Standard Security Integration**: All frontends use the `cmdutil` package's `RegisterSecurityFlags` to ensure consistent mTLS/PKI configuration and identity handling.
- **Implementations**:
  - **TUI (terminal)**: A fully-featured, Vim-inspired client using `bubbletea` with integrated Modal editing and Omni-palette search.
  - **GUI (Wayland/X11)**: A high-performance native client built with `Gio`, supporting hardware acceleration and advanced system-theme integration.
  - **Web**: A modern web client providing access via a Go-WASM bridge, enabling remote workspace collaboration in the browser.

---

## 5. Bootstrapping and Configuration (Startup Flow)

Alloy follows a **"Frontend-Driven" bootstrapping model**.

1. **Frontend Init**: The frontend loads its local configuration (paths, certs, policy, plugins).
2. **Backend Detection**: It checks for an existing backend at the configured socket/address.
3. **Instance Creation (Optional)**: If no backend is found and configuration permits, the frontend launches a new backend process.
4. **Provisioning**: The frontend passes a configuration payload to the newly started backend. This payload includes components, security policies, and IPC settings.
5. **Connection**: Once the backend is ready, the frontend connects and registers its identity.
