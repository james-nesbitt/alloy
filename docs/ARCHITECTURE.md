# Alloy Architecture Design: Pragmatic Hybrid Kernel

Alloy's architecture follows a **Pragmatic Hybrid Kernel** pattern. Unlike a traditional micro-kernel where all services are external, Alloy integrates its most critical "infrastructure" services—**IAM, KV, Events, and Telemetry**—directly into the Go-based core. This provides the performance and integrity of a monolith for low-level system operations while preserving the isolation and extensibility of a micro-kernel for application logic.

## 1. Core Architecture

### 1.1 The Integrated Kernel
The Go kernel is responsible for the core lifecycle and the "Operating System" layer of the workspace. Its responsibilities are split between its internal Go-native logic and the services it exposes to external plugins.

#### Integrated Core Services:
- **Identity & Access Management (IAM)**: A native Go implementation that manages identities, roles, and policies. It serves as the authoritative Policy Decision Point (PDP) for the entire system.
- **Key-Value Store (KV)**: A high-performance, persistent state store integrated directly into the kernel to eliminate RPC overhead for frequent metadata reads/writes.
- **Event Bus (Pub/Sub)**: The internal central nervous system for message routing and asynchronous event notification.
- **Telemetry & Logging**: Native monitoring of all kernel and plugin interactions.

#### Host Functions:
- **Message Bus (IPC)**: A high-performance message routing system for cross-plugin and frontend-to-backend communication.
- **WASM Plugin Engine**: A `wazero`-based runtime for loading and executing WASM-based components using the WASM Component Model (WIT).
- **Service Discovery**: Allows plugins to discover the WIT-compatible surface of the integrated Core Services.

### 1.2 Communication Paths
Alloy features two distinct communication models:
1. **Direct Go Calls (Internal)**: Internal kernel components communicate via direct method calls. This ensures high throughput and zero-latency for the "Zero-Latency Core."
2. **WIT-based Async Messaging (Plugins/Frontends)**: All 3rd-party code and frontends interact via the Message Bus. The kernel converts internal Go results into the asynchronous, WIT-compatible response format.

## 2. Application Logic (WASM Plugins)

While infrastructure is integrated, all **application-level logic** is offloaded to isolated WASM plugins. 

- **Isolation**: Each plugin runs in a `wazero` sandbox, ensuring that a bug in an AI Agent or Chat plugin cannot crash the core kernel or access unauthorized data.
- **Hot-Reloading**: The `WasmManager` can swap a plugin binary at runtime while preserving the registry and subscriber state.
- **Standard WIT Surface**: Every plugin communicates with the integrated Core Services via standard WebAssembly Interface Type (WIT) bindings. For example, a plugin's call to `alloy:kv/read` is fulfilled by the kernel's integrated KV store.
- **Discovery**: The kernel can automatically discover WASM plugins in directories specified via the `--wasm-plugins` flag.
- **Resource Constraints**: Each plugin is assigned a sandbox profile defining limits on memory, CPU "fuel," and internal storage access.

## 3. Frontends

Frontends provide user interaction and connect to the backend core via IPC. See [Alloy Frontends](FRONTENDS.md) for detailed implementation details and build requirements.
- **Connection Model**: Frontends can connect to an already-running backend or initialize a new one.
- **Multi-Client**: Multiple frontends can connect to a single backend simultaneously.
- **Bootstrapping Process**:
    - **Self-Sourced Configuration**: Each frontend is responsible for loading its own configuration.
    - **Backend Orchestration**: When a frontend starts a new backend, it is responsible for passing the initial backend configuration, including the list of plugins to load and security policy restrictions.
    - **Policy Integration**: Backend and plugin policy restrictions are defined within the configuration and enforced by the kernel upon startup.
- **Implementations**:
  - **TUI (terminal)**: Written in Go (e.g., bubbles/bubbletea).
  - **GUI (Wayland)**: Written in Go (e.g., Fyne or Gio).
  - **Web**: Go-based web server serving a JS/TS frontend.

## 4. Bootstrapping and Configuration

Alloy follows a "Frontend-Driven" bootstrapping model for new backend instances.

### 4.1 Startup Flow
1. **Frontend Init**: The frontend loads its local configuration (paths, certs, policy, plugins).
2. **Backend Detection**: It checks for an existing backend at the configured socket/address.
3. **Instance Creation (Optional)**: If no backend is found and configuration permits, the frontend launches a new backend process.
4. **Provisioning**: The frontend passes a configuration payload to the newly started backend. This payload includes:
    - **Components**: Definitions and paths for core kernel modules and WASM plugins.
    - **Security Policies**: MAC (Mandatory Access Control) rules for plugins and users.
    - **IPC Settings**: Port/Socket definitions for the message bus.
5. **Connection**: Once the backend is ready, the frontend connects and registers itself by providing its identity in the `Sender` field of its initial messages.

### 4.2 Configuration Ownership
While the initiating frontend provides the initial state to a backend, that backend then becomes a shared resource. Subsequent frontends connecting to it must adhere to the policy established during the initial provision.
