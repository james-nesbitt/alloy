# Alloy Architecture: Integrated Services & Plugins

Alloy distinguishes between **Integrated Core Services**, which provide the "Operating System" layer of the workspace, and **Extensible Plugins**, which provide decoupled application logic.

## 1. Integrated Core Services (Go Native)

These services are built directly into the Alloy kernel to ensure zero-latency performance, system integrity, and a guaranteed bootstrap sequence. They are always available and cannot be disabled.

| Service ID | Type | Description |
|------------|------|-------------|
| `iam` | Native | **Identity & Access Management**: Native Go RBAC engine. |
| `kv` | Native | **Key-Value Store**: Integrated state persistence for all components. |
| `events` | Native | **Event Bus**: The central message routing and Pub/Sub backbone. |
| `telemetry`| Native | **Monitoring**: Integrated logging, tracing, and health metrics. |
| `registry` | Native | **Service Discovery**: Manages the mapping of WIT capabilities to targets. |

## 2. Extensible Application Plugins (WASM)

Application logic is implemented as independent WebAssembly binaries using the **WIT-based Component Model**. This is the preferred way to extend Alloy's functionality.

| Plugin ID | Type | Description |
|-----------|------|-------------|
| `ai` | WASM | LLM integration, tool-use orchestration, and AI workflows. |
| `buffer` | WASM | Manages collaborative data streams and shared editing state. |
| `chat` | WASM | Real-time messaging, channels, and presence tracking. |
| `index` | WASM | **Knowledge Graph Indexer**: Unified activity indexing and team search. |
| `omni-palette`| WASM| **Unified Search**: Single entry point for commands, files, and knowledge. |
| `project` | WASM | Organizes resources (buffers, chats, files) into logical projects. |
| `secrets` | WASM | Encrypted storage and policy-based retrieval of sensitive data. |
| `tasks` | WASM | Background job management and scheduling. |
| `health-wasm`| WASM| High-level application diagnostics (complements core telemetry).|

## 3. Detailed Service & Plugin Descriptions

### 3.1 Integrated IAM (`iam`)
Provides the security backbone for the system.
- **Immediate Enforcement**: Operates as a native kernel interceptor, blocking unauthorized calls *before* they exit the kernel's routing loop.
- **RBAC**: Manages roles and permissions for system users, frontends, and WASM plugins.
- **Zero-Latency**: Authorization checks happen at native Go speeds with no message serialization.

### 3.2 AI Agent (`ai`)
The interface for Large Language Models.
- **Tooling**: Allows the AI to interact with integrated services (e.g., asking the `project` plugin for a list of files).
- **Context**: Manages context windows and history for AI interactions.

### 3.3 Buffer Manager (`buffer`)
Handles all data that needs to be synchronized across frontends.
- **Shared State**: Multiple frontends can open the same buffer for collaborative work.
- **Streaming**: Broadcasts logs or metrics to connected clients using the integrated Event bus.
- **Persistence**: Interfaces with the core `kv` service to save/load buffer data.

## 4. Development Guidelines

When creating a new plugin:
1. **Use the SDK**: Always use `github.com/james-nesbitt/alloy/pkg/wasm/guest` for a consistent experience.
2. **Standardize Naming**: Use short, lowercase IDs without prefixes like `plugin-` or `-manager`.
3. **Define Capabilities**: Explicitly register capabilities in the `AlloyInit` block so the system can discover your plugin's features.
4. **Message Safety**: Use the `AlloyMessage` struct and its helper methods for IPC.

For technical build instructions, refer to [Build and Plugin Guidelines](BUILD_AND_PLUGINS.md).
