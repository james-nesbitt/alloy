# Alloy Development: Build and Plugin Guidelines

## 1. Build System

All build artifacts (binaries and WASM modules) are centralized in the `/build` directory. This keeps the source tree clean and simplifies deployment and CI/CD pipelines.

### 1.1 Directory Structure
- `/build/core`: The main Alloy backend (kernel) binary.
- `/build/frontend`: The Alloy CLI/TUI frontend binary.
- `/build/wasm/`: Directory containing all compiled WASM plugins (e.g., `chat.wasm`, `secrets.wasm`).

### 1.2 Compilation
Use the [justfile](../justfile) to manage builds:
- `just build-all`: Compiles the core, frontend, and all WASM plugins.
- `just build-core`: Compiles only the backend.
- `just build-wasm`: Compiles all WASM plugins using `build_plugins.sh`.

## 2. Plugin Architecture: Native vs. WASM

Alloy supports two types of plugins: **Native (Go)** and **WASM**. Choosing the correct type is critical for maintaining the system's security, performance, and portability.

### 2.1 Native Plugins
Native plugins are compiled directly into the Alloy core binary.

**When to use Native:**
- **Core Infrastructure**: Components that provide the fundamental "plumbing" of the system (e.g., the Event Bus, KV Store abstraction).
- **Resource Management**: Logic that requires direct, high-performance access to host system resources (memory, file descriptors) that WASI cannot yet provide efficiently.
- **Bootstrapping**: Components necessary for the kernel to reach a state where it can safely load other plugins.

**Decision Criteria:**
- Does it provide a service used by almost every other plugin?
- Is it part of the "Trusted Computing Base" (TCB)?

### 2.2 WASM Plugins
WASM plugins are standalone `.wasm` files loaded dynamically by the kernel's runtime (`wazero`).

**When to use WASM:**
- **Application Logic**: Business rules, chat protocols, AI agent behaviors, and user-level features.
- **Untrusted/Third-Party Code**: Any logic that should not have the power to crash the kernel or access unauthorized host files.
- **Security-Sensitive Logic**: Components like **IAM** or **Secrets Management**—by running these in WASM, we ensure they are isolated from the rest of the kernel and other plugins.

**Decision Criteria:**
- Does it handle user data?
- Does it need to be sandboxed?
- Should it be swappable or updateable without restarting the kernel?

### 2.3 Summary Comparison

| Feature | Native | WASM |
| :--- | :--- | :--- |
| **Isolation** | None (Runs in Kernel space) | High (Sandboxed) |
| **Security** | Trusted | Untrusted / Restricted |
| **Performance** | Native (Highest) | Near-native (Wasm Overhead) |
| **Language** | Go only | Any language targeting WASM/WASI |
| **Updateability** | Requires recompiling Core | Dynamic reload |
| **Access** | Full Host Access | Capability-based (WASI/Host Calls) |

## 3. Current Decision Tree

1. **Is it the Message Bus or KV Provider?** -> **Native**
2. **Is it a user-facing feature?** (Chat, AI, Buffer) -> **WASM**
3. **Is it a security service?** (IAM, Secrets) -> **WASM** (for isolation/auditability)
4. **Is it a monitoring service?** (Health, Tasks) -> **WASM** (to prevent crashes from affecting core)
