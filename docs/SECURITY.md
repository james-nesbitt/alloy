# Alloy Security Framework

Alloy enforces a **"Security-First"** architecture with built-in Mutual TLS (mTLS) and Peer-to-Peer identity verification at every layer.

## 1. Identity Management & PKI

Alloy uses a **Connection-Level Identity** model from its internal Public Key Infrastructure.
- **Mutual TLS (mTLS)**: Every connection (TCP or Unix) requires a client certificate signed by the Alloy root CA. The Common Name (CN) or Serial is automatically extracted as the immutable `Actor` for all subsequent messages.
- **Peer Verification**: On Unix Sockets, `PeerCredentials` (UID/GID) are used to confirm local client identity and map them to their configured system actor.
- **Verification**: The `Sender` field in a message provides the claimed identity, while the `Actor` field (populated by the kernel) provides the *verified* identity used for all authorization decisions.

## 2. Secure Communication

- **Encrypted Channels**: All IPC traffic is encrypted with modern TLS 1.3 suites.
- **Unified Security Flags**: Frontends and the kernel share a common `cmdutil` security suite (`RegisterSecurityFlags`) for certificate, socket, and actor configuration.
- **PKI Lifecycle**: Simple CLI commands via the `alloy` tool manage the local Root CA, backend certificates, and user/frontend keys.

## 3. Mandatory Access Control (MAC)

The Alloy kernel acts as both the **Policy Enforcement Point (PEP)** and the **Policy Decision Point (PDP)** through a tiered security architecture.

### 3.1 Tier 1: Native Kernel Router Protection
- **Low-Latency Authorization**: Every message routed by the kernel is intercepted by a native Go-based security layer (`pkg/kernel/iam.go`).
- **Integrated RBAC Policy**: High-level permissions are managed within the kernel's state for core service access. 
- **System Integrity**: Internal kernel components (`kernel`, `events`, `iam-native`) are privileged to prevent deadlocks.

### 3.2 Tier 2: Pluggable WASM IAM (Advanced RBAC)
- **Granular Control**: Advanced security logic (e.g., resource-specific rules like `buffer:write:public-*`) is handled by the `iam` WASM plugin.
- **Dynamic Policy Management**: Roles and permissions can be updated on-the-fly without kernel restarts.
- **Persistence**: Policies and identities are stored in the `alloy-kv` store.
- **Active Auditing**: The IAM plugin maintains security health metrics and emits audit events to the `events` service for real-time monitoring.

### 3.3 WASM Sandboxing
- **Isolation**: External application logic remains isolated in WASM sandboxes. Plugins cannot bypass the hardware-enforced boundaries of the `wazero` runtime or access the kernel's internal services without authorized WIT calls, each of which is validated.

## 4. Auditing

- **Security Monitor**: The `iam` plugin provides a dashboard widget that tracks allowed and denied access attempts.
- **Audit Events**: Every authorization check emits a `system:audit` event, allowing for external security monitoring and log aggregation.
