# Alloy Security Framework

Alloy is designed with a "Security-First" mindset, though several high-level security features (like mTLS and encrypted envelopes) are currently deferred during the initial development phase.

## 1. Identity Management

Alloy uses a **Connection-Level Identity** model. 
- **Automated Identification**: The IPC server automatically extracts identity from the connection (e.g., mTLS Subject Common Name or peer UID/GID) and stamps it as the `Actor` on every message.
- **Verification**: The `Sender` field in a message provides the claimed identity, while the `Actor` field (populated by the kernel) provides the *verified* identity used for authorization.

## 2. Secure Communication

- **Mutual TLS (mTLS)**: Cross-process communication over TCP requires signed certificates.
- **Local Hardening**: PeerCredential verification (UID/GID) is used for Unix Domain Sockets to confirm the identity of local clients.
- **Insecure Development Mode**: A bootstrap mode is available for local testing where identity is inferred but skip-validated.

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
