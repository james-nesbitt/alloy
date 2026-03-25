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

The Alloy kernel acts as both the **Policy Enforcement Point (PEP)** and the **Policy Decision Point (PDP)** through its integrated **IdentityManager**.

- **Integrated IAM Interceptor**: Every message routed by the kernel is intercepted by a native Go-based security layer. The kernel performs an instantaneous, zero-latency authorization check using the integrated `IdentityManager`.
- **Integrated RBAC Policy**: Permissions are managed as Role-Based Access Control (RBAC) rules stored within the kernel's state. 
- **System Integrity (Bypass)**: Internal kernel components (`kernel`, `events`, `iam`) are privileged to ensure system stability and prevent deadlocks during security-critical operations.
- **WASM Sandboxing**: External logic remains isolated in WASM sandboxes. Plugins cannot bypass the hardware-enforced boundaries of the `wazero` runtime or access the kernel's internal services without authorized WIT calls, each of which is validated.

## 4. Auditing

- **Audit Logging**: Major kernel actions, including routing and registration, are logged to the standard output and will eventually be moved to a tamper-evident audit log file.
