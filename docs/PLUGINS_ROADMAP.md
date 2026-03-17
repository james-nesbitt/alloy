# Alloy Plugin Roadmap

This document describes the planned core plugins for the Alloy platform. These plugins provide the functional logic of the system while remaining isolated from the micro-kernel.

## 1. Buffer Manager (`plugin-buffer-manager`)
The Buffer Manager handles shared content and data streams across multiple frontend instances.

- **Shared State**: Allows frontends to open the same "buffer" (text, code, binary) and perform concurrent edits.
- **Data Streaming**: Provides a mechanism to broadcast high-volume data like log streams, metrics, or video data to multiple connected frontends simultaneously.
- **Synchronization**: Implements conflict resolution or locking for collaborative editing.
- **Persistence**: Interfaces with storage to save and load buffer states.

## 2. Identity & Access Manager (`plugin-iam`)
This plugin manages users, teams, and roles, providing granular control over what different connections can do.

- **RBAC/ABAC**: Implements Role-Based or Attribute-Based Access Control.
- **SSO Integration**: Provides hooks for external identity providers (OIDC, SAML, GitHub, Google).
- **Session Management**: Tracks active users across multiple frontend connections.
- **Policy Provider**: Feeds the Backend Kernel with the necessary permissions for message routing authorization.

## 3. Group Chat (`plugin-chat`)
Enables real-time communication between users and frontends.

- **Direct Messaging**: One-to-one secure messaging between users.
- **Rooms/Channels**: Support for persistent or ephemeral group chat rooms.
- **Presence**: Tracks user online/offline status.
- **History**: Interfaces with the Buffer Manager or a database to provide message history and search.

## 4. AI Agent (`plugin-ai-agent`)
Provides an interface for frontends to interact with Large Language Models and other AI tools.

- **Query Interface**: Standardized API for frontends to send prompts and receive responses.
- **Context Management**: Manages conversation history and "memory" for AI sessions.
- **Tool Use (Functions)**: Allows the AI agent to interact with other plugins (e.g., "AI, read the latest logs from the Buffer Manager").
- **Provider Agnostic**: Supports multiple backends (OpenAI, Anthropic, local Llama via Ollama, etc.).

## 5. Local Storage (`plugin-storage`)
Provides persistent storage services to other plugins and frontends, abstracting the physical filesystem.

- **Key-Value Store**: Simple API for plugins to save state and configuration.
- **File Management**: Handles the storage of binary blobs and large files.
- **Gatekeeper**: Enforces storage quotas and ensures plugins only access their allowed namespaces.
- **Search**: Provides indexing and search capabilities for stored content.

## 6. Project Manager (`plugin-project-manager`)
Acts as the organizational layer, grouping disparate resources into high-level "Projects" or "Topics."

- **Resource Grouping**: Associates specific Buffers, Chat Rooms, and Files with a Project ID.
- **Context Switching**: Allows a frontend to load all resources related to a project in one action.
- **Metadata**: Stores project-specific descriptions, tags, and status.
- **Dependency Orchestrator**: Interacts with the IAM plugin to manage project-level permissions and with the Storage plugin for persistence.

## 7. Command Manager (`plugin-command-manager`)
Provides a centralized registry for discovering and executing actions across the system.

- **Command Registration**: Frontends and plugins can register their own commands (e.g., `git:commit`, `buffer:save`).
- **Discovery**: Components can query the Command Manager to find available actions, including their expected arguments and descriptions.
- **Hierarchical Namespacing**: Commands are organized in a `domain:category:action` hierarchy.
- **Execution Orchestration**: Routes command execution requests to the providing component via the kernel's message bus.
- **Auditing**: Every command execution is logged for security and debugging.

## 8. Registry & Plugin Manager (`plugin-registry-manager`)
Manages the lifecycle of plugin artifacts and handles the logistics of keeping the system up to date.

- **Plugin Acquisition**: Downloads WASM plugin binaries from remote registries (e.g., OCI-compliant registries or HTTPS endpoints).
- **Compilation/Toolchain**: Optionally manages the compilation of plugins from source if a supported toolchain is available.
- **Lifecycle Coordination**: Signals the Backend Kernel to load, unload, or hot-reload plugins.
- **Versioning & Upgrades**: Tracks plugin versions, checks for updates, and manages the upgrade process to ensure compatibility (interacting with the ABI version checks).
- **Uninstall/Cleanup**: Safely removes plugins and cleans up their associated virtual filesystem data (working with the Storage plugin).

---

# Guidance for Plugin Development

When implementing these or new plugins, follow these principles:
- **Kernel-Managed Lifecycle**: Plugins are started and stopped by the Backend Kernel. They should be designed to handle clean shutdowns and rapid startups.
- **Isolation**: Each plugin is a standalone WASM module. It cannot access the memory of the kernel or other plugins directly.
- **Local State Management**:
    - Plugins are the **sole source of truth** for their logic domain (e.g. the Git plugin owns "current branch").
    - **Persistence**: The kernel manages plugin state for loading/initializing via `SaveState` and `LoadState`.
    - **Host KV**: The kernel provides host functions for plugins to save, load, and cache their internal data blocks without managing physical files.
- **Communication via Commands & Events**:
    - **Pull**: External components query state by calling plugin-advertised commands.
    - **Push**: Plugins broadcast state changes via the `TypeEvent` message type.
- **Plugin Inter-dependency**:
    - **Explicit Dependencies**: A plugin can require the presence of another (e.g., Chat requires IAM for permissions; Project Manager requires Storage and Buffer Manager).
    - **Optional Capabilities**: Gracefully degrade or enhance functionality based on available plugins (e.g., AI Agent enables "save to project" only if the Project Manager is active).
    - **Discovery**: Use the kernel's service discovery mechanisms to check for available plugin targets.
- **Granular Permissions**: Define specific methods that can be restricted by the IAM plugin.
- **Performance**: Remember that WASM execution has overhead. For high-throughput data (video/logs), focus on efficient buffer passing.
