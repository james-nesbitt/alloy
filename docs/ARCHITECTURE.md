# Alloy Architecture Design

Alloy's architecture follows a micro-kernel pattern, where the core (backend) provides a message-passing infrastructure and a set of minimal services while offloading actual application logic to plugins and frontends.

## 1. Core Architecture

### 1.1 Backend Kernel
The backend kernel is responsible for:
- **Message Bus (IPC)**: A high-performance message routing system for cross-plugin and frontend-to-backend communication.
- **WASM Plugin Engine**: Runtime for loading and executing WASM-based plugins.
- **Security & Policy Engine**: Manages user authentication, authorization, and plugin sandbox constraints.
- **Service Discovery**: Allows plugins and frontends to discover available capabilities.
- **Logging & Monitoring**: Centralized logging for the kernel itself and all managed plugins.

### 1.2 IPC Mechanism
- **Socket-Based**: Communication occurs over Unix Domain Sockets (local) or TCP (network).
- **Transport**: Plain TCP/Unix sockets are used for transport (encryption and authentication are currently deferred).
- **Message & Event Bus**: 
    - Supports **Request/Response** patterns for direct interactions.
    - Acts as an **Event System (Pub/Sub)**, allowing components to emit events (`TypeEvent`) and others to subscribe to them.
- **Message Format**: Encoded as newline-delimited JSON (JSON-seq) passing through the message bus.
- **Multi-user/Multi-tenant**: Supports multiple users with distinct contexts and permissions.

## 2. Plugins (WASM)

Plugins extend the capabilities of the Alloy backend.
- **Isolation**: Each plugin runs in its own WASM sandbox (e.g., using `wazero`). Each plugin is a separate WASM binary.
- **Lifecycle Management**: 
    - The Backend Kernel provides the low-level mechanism for starting, stopping, and restarting plugins.
    - High-level orchestration (downloading, upgrading, and managing the plugin catalog) is handled by the **Registry & Plugin Manager** plugin (see [Plugin Roadmap](PLUGINS_ROADMAP.md)).
- **Standard Interface**: Plugins must implement a standard interface for message handling.
- **Inter-Plugin Communication**: Plugins can communicate with each other via the kernel's message bus.
- **Dependencies**: 
    - Plugins can depend on the presence of other plugins to function.
    - Plugins can provide **optional functionality**; for example, the Chat plugin might offer enhanced logging only if the Buffer Manager plugin is available.
- **Runtime Specifications**:
    - **Memory Management**: Uses a "Copy-on-pass" model for small messages and "Reference IDs" for large data (video/logs) to avoid expensive copying between host and guest.
    - **Resource Constraints**: Each plugin is assigned a "Sandbox Profile" defining maximum memory and CPU "fuel" (instruction counts) to prevent resource exhaustion.
    - **Storage**: Plugins use a Virtual Filesystem (WASI) scoped to a specific, kernel-managed directory.
    - **Concurrency**: Host-calls are non-blocking where possible, ensuring plugins remain responsive to the message bus.
    - **Versioning**: Uses a SemVer-based ABI. Plugins must declare their ABI version and required capabilities in a manifest.
- **Examples**: Core functionality like buffer management, user roles, chat, and AI agents are implemented as plugins (see [Plugin Roadmap](PLUGINS_ROADMAP.md)).
- **SDK**: A core library in Go/AssemblyScript/Rust for developing plugins.

## 3. Frontends

Frontends provide user interaction and connect to the backend core via IPC.
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
3. **Instance Creation (Optional)**: If no backend is found and configuration permits, the frontend launches a 새로운 backend process.
4. **Provisioning**: The frontend passes a configuration payload to the newly started backend. This payload includes:
    - **Components**: Definitions and paths for core kernel modules and WASM plugins.
    - **Security Policies**: MAC (Mandatory Access Control) rules for plugins and users.
    - **IPC Settings**: Port/Socket definitions for the message bus.
5. **Connection**: Once the backend is ready, the frontend connects and registers itself by providing its identity in the `Sender` field of its initial messages.

### 4.2 Configuration Ownership
While the initiating frontend provides the initial state to a backend, that backend then becomes a shared resource. Subsequent frontends connecting to it must adhere to the policy established during the initial provision.
