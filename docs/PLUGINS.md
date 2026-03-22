# Alloy Plugins

Alloy is a highly modular system where nearly all functional logic is implemented as independent plugins. This document describes the core plugins, their responsibilities, and the standardized naming conventions used in the system.

## 1. Plugin Architecture

Alloy supports two types of plugins:
- **Native Plugins**: Compiled directly into the core or loaded as Go plugins (used for low-level infrastructure like storage or networking).
- **WASM Plugins**: Independent WebAssembly binaries using the **WIT-based Component Model**. This is the preferred way to extend Alloy.

## 2. Standardized Core Plugins

The following plugins are part of the standard Alloy distribution:

| Plugin ID | Type | Description |
|-----------|------|-------------|
| `ai` | WASM | LLM integration, tool-use orchestration, and AI-assisted workflows. |
| `buffer` | WASM | Manages shared data streams and concurrent editing buffers. |
| `chat` | WASM | Real-time messaging, channels, presence tracking, and direct messages. |
| `health` | WASM | Resource monitoring, heartbeats, and system telemetry. |
| `iam` | WASM | Identity and Access Management; handles RBAC and user sessions. |
| `project` | WASM | Organization of resources (buffers, chats, files) into logical projects. |
| `secrets` | WASM | Encrypted storage and policy-based retrieval of sensitive data. |
| `tasks` | WASM | Background job management, scheduling, and to-do tracking. |

## 3. Detailed Plugin Descriptions

### 3.1 Buffer Manager (`buffer`)
Handles all data that needs to be synchronized across frontends.
- **Shared State**: Multiple frontends can open the same buffer for collaborative work.
- **Streaming**: Broadcasts logs or metrics to connected clients.
- **Persistence**: Interfaces with the kernel to save/load buffer data.

### 3.2 AI Agent (`ai`)
The interface for Large Language Models.
- **Tooling**: Allows the AI to interact with other plugins (e.g., asking the `project` plugin for a list of files).
- **Context**: Manages context windows and history for AI interactions.

### 3.3 Identity & Access Manager (`iam`)
Provides the security backbone for the system.
- **Authorization**: Validates if a user/frontend has permission to send a specific message to a target.
- **RBAC**: Manages roles and permissions.

### 3.4 Project Manager (`project`)
Provides a layer of organization above individual resources.
- **Logical Grouping**: Groups related buffers, chat channels, and tasks.
- **Persistence**: Remembers the last active project and restores its state on boot.

## 4. Development Guidelines

When creating a new plugin:
1. **Use the SDK**: Always use `github.com/james-nesbitt/alloy/pkg/wasm/guest` for a consistent experience.
2. **Standardize Naming**: Use short, lowercase IDs without prefixes like `plugin-` or `-manager`.
3. **Define Capabilities**: Explicitly register capabilities in the `AlloyInit` block so the system can discover your plugin's features.
4. **Message Safety**: Use the `AlloyMessage` struct and its helper methods for IPC.

For technical build instructions, refer to [Build and Plugin Guidelines](BUILD_AND_PLUGINS.md).
