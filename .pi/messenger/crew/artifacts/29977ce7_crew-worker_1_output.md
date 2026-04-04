I have completed **task-4: Headless Architecture & Zero-Frontend Clients**.

### Summary of Changes
- **IAM & API Extension**: Added `Headless` boolean flags to `api.Registration` and `api.PluginMetadata` in `api/messages.go` to distinguish between graphical and automated actors.
- **Kernel Infrastructure**: Updated `pkg/kernel/kernel.go` to track the "headless" status of registered frontends and skip visual-only state pushes (like initial widget synchronization) for headless clients.
- **IPC Support**: Modified `pkg/ipc/server.go` to extract a `headless` flag from message metadata, allowing clients to identify as headless during the initial connection handshake.
- **Coordination & Synchronization**:
    - Added `Target string` to the `api.Intent` struct to support directed delegation (coordinated with **task-3**).
    - Added `VisualIntent` method to the `SharedBuffer` interface to allow headless actors to broadcast virtual cursors and highlights (coordinated with **task-5**).
    - Injected the `EventManager` into the `BufferManager` in the kernel initialization to enable buffer-level event broadcasting (coordinated with **task-5**).

### Verification
- Verified the kernel and IPC changes with unit tests: `go test ./pkg/kernel/...` and `go test ./pkg/ipc/...`.
- Confirmed that the global build passes with the new interface changes: `just build-all`.
- Coordinated with `SwiftViper`, `PureJaguar`, and `ZenArrow` to ensure interface changes were implemented as needed for their concurrent tasks.

Task **task-4** is marked as complete. All file reservations have been released.