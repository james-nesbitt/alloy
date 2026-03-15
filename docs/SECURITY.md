# Alloy Security Framework (SIPC)

Alloy uses a Secure IPC (SIPC) framework to ensure that all communications between the kernel, plugins, and frontends are authenticated, encrypted, and auditable.

## 1. Identity & PKI Management

Alloy implements a private PKI (Public Key Infrastructure) to manage component identities.

### 1.1 The Root of Trust
- **Root CA**: A private Root Certificate Authority is generated during the initial setup (`alloy-cli security init`).
- **Storage**: The CA private key is stored securely (0600 permissions), while the public key is embedded in all core binaries.

### 1.2 Certificate Issuance
- Every component (Backend, TUI, GUI, Web Server) must possess a certificate signed by the Alloy Root CA.
- **Client Certificates**: Include metadata identifying the component's role and unique ID.
- **Generation**: Managed via the CLI: `alloy-cli security gen-client --name "component-name"`.

## 2. Secure Communication (mTLS)

All IPC, whether over Unix Domain Sockets or Network TCP, is secured using **Mutual TLS (mTLS)**.

- **Frontend-to-Backend**: The backend requires and verifies client certificates (`tls.RequireAndVerifyClientCert`).
- **Verification**: Both parties verify the other's certificate against the embedded Root CA public key.
- **Local Hardening**: For Unix sockets, the backend also verifies `PeerCredentials` (UID/GID) to ensure process-level ownership matches the certificate identity.

## 3. Connection Handshake Process

1. **Discovery**: Frontend identifies the Backend's socket path or address.
2. **mTLS Handshake**: Secure tunnel establishment and identity verification.
3. **Registration**: Frontend sends a `system.register` message over the secure tunnel.
4. **Policy Check**: Backend evaluates the component's identity against the security policy.
5. **Auditing**: The connection attempt, identity, and result are recorded in the audit log.

## 4. Message Security & Auditing

### 4.1 Secure Message Envelope
Messages may be wrapped in a secure envelope to include integrity signatures and tracking metadata:
- **TraceID**: For cross-component debugging and auditing.
- **Signature**: Ensures message integrity between the source and the kernel.

### 4.2 Mandatory Access Control (MAC)
The Kernel serves as the Policy Enforcement Point (PEP).
- **Authorization**: Every message is checked against a policy before routing.
- **Policy Source**: For a given backend instance, the initial security policy is provided by the bootstrapping frontend as part of the startup configuration.
- **Sandboxing**: WASM plugins are restricted to specific "host calls" and cannot access system resources directly.

### 4.3 Tamper-Evident Auditing
- **Audit Log**: A dedicated, structured log file (`audit.log`).
- **Content**: Records all connection events, authentication results, and authorized/denied message routes.
- **Integrity**: Log entries are structured to support future hashing chains to detect log tampering.
