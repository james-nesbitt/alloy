# Hardware-Verified Identity (mTLS)

Integrate TPM or Secure Enclave support for mTLS key management. Update pkg/security/pki to support hardware-backed key generation and certificate signing. Enhance pkg/ipc/mtls.go to utilize these hardware-stored keys for kernel-to-actor and actor-to-actor communication.
