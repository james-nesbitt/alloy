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

The Kernel serves as the **Policy Enforcement Point (PEP)**, with the `iam` plugin acting as the **Policy Decision Point (PDP)**.
- **IAM Interceptor**: Every message routed by the kernel is intercepted. The kernel performs a synchronous call to the `iam` plugin to verify if the `Actor` is authorized to call the requested `Method` on the `Target`.
- **RBAC Policy**: Permissions are managed as Role-Based Access Control (RBAC) rules within the `iam` plugin.
- **System Bypass**: Internal system components (`kernel`, `events`, `command-manager`) have a hard-coded bypass to prevent circular dependencies and ensure system stability.
- **Sandboxing**: WASM plugins provide memory-isolated sandboxes. They cannot access the host filesystem or network except through authorized IPC calls validated by the IAM interceptor.

## 4. Auditing

- **Audit Logging**: Major kernel actions, including routing and registration, are logged to the standard output and will eventually be moved to a tamper-evident audit log file.
