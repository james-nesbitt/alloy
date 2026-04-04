I have implemented the hardware-verified mTLS identity as specified in **task-6**.

## Summary of Changes

1.  **Hardware-Backed Key Support**:
    -   Defined `HardwareProvider` and `HardwareSigner` (extending `crypto.Signer`) interfaces in `pkg/security/pki/pki.go`.
    -   Updated `KeyPair` to use `crypto.Signer`, allowing for both `ecdsa.PrivateKey` (software) and hardware-backed signers.
    -   Added a custom `ALOY HARDWARE KEY` PEM type with metadata headers for provider identification.
    -   Modified `SignCertificate`, `ParseKeyPair`, and `EncodeToPEM` to handle hardware-backed keys.

2.  **Seamless TLS Integration**:
    -   Added `LoadTLSCertificate` to `pki` to transparently load both software and hardware keys into `tls.Certificate`, allowing existing mTLS logic to work without major refactoring.
    -   Updated `pkg/ipc/mtls.go` and `pkg/security/identity/store.go` to use this new loading mechanism.

3.  **Verification**:
    -   Expanded `pkg/security/pki/pki_test.go` with `TestHardwarePKI` using a mock hardware provider.
    -   Added `pkg/ipc/mtls_test.go` to verify mTLS configuration with hardware keys.
    -   Fixed a minor regression in `pkg/ipc/ipc_test.go`.
    -   Verified that all related tests pass: `go test ./pkg/security/pki ./pkg/ipc/... ./pkg/security/identity/...`.

## Evidence
- Commits: `cf9858d`
- Tests: `go test -v ./pkg/security/pki/ ./pkg/ipc/mtls_test.go` (Hardware-specific cases PASS)

I have also updated **ZenArrow** (working on task-7) that the dependencies are met.