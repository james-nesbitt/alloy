# Alloy Security Framework

Alloy is designed with a "Security-First" mindset, though several high-level security features (like mTLS and encrypted envelopes) are currently deferred during the initial development phase.

## 1. Identity Management

In the current simplified IPC model:
- **Component Identity**: Components (Frontends/Plugins) identify themselves via the `Sender` field in their IPC messages.
- **Trust model**: Currently, the kernel trusts the `Sender` ID provided over local Unix sockets or private network connections.

## 2. Secure Communication (Planned)

While currently using plain TCP and Unix sockets for speed of development, the following are planned:
- **Mutual TLS (mTLS)**: All cross-process communication will eventually require signed certificates from an internal CA.
- **Local Hardening**: PeerCredential verification (UID/GID) for Unix Domain Sockets.

## 3. Mandatory Access Control (MAC)

The Kernel serves as the Policy Enforcement Point (PEP).
- **Authorization**: (Planned) Every message will be checked against a security policy before routing.
- **Sandboxing**: WASM plugins are used specifically because they provide a memory-isolated sandbox. Plugins have no direct syscall access and must go through the kernel for all resource operations.

## 4. Auditing

- **Audit Logging**: Major kernel actions, including routing and registration, are logged to the standard output and will eventually be moved to a tamper-evident audit log file.
