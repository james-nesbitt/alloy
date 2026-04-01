# ARCHITECT DESIGN: PHASE 6 (FOUNDATION HARDENING)

## 1. Intent-Centric Modal Engine (V2)

The current `ModalEngine` is a prototype. We must evolve it into a production-grade grammar engine.

### 1.1 State Machine Evolution
The `modal.State` will now track key sequences and prefix counts explicitly.

```go
type State struct {
    Mode      Mode
    Pending   []Key    // Multi-key sequence (e.g., ['g', 'g'])
    Count     int      // Prefix argument (e.g., 5 in '5j')
    Register  string   // Active register for yank/paste
    Selection Selection
}
```

### 1.2 Driver Interface (Incomplete Sequences)
Drivers must signal when a key is part of an incomplete sequence (e.g., 'g' in Vim).

```go
type Result struct {
    Intent     Intent
    Consumed   bool
    Incomplete bool // True if sequence is waiting for more keys
}

type Driver interface {
    Handle(key Key, state *State) Result
}
```

### 1.3 Universal Intent Schema
We define a decoupled `Intent` tree that any frontend (TUI, GUI, Web) can implement:

| Intent Category | Verbs / Actions |
| :--- | :--- |
| **Motion** | `up`, `down`, `line-start`, `buffer-end`, `jump-tag` |
| **Window** | `split-v`, `split-h`, `close`, `focus-left` |
| **Buffer** | `save`, `reload`, `format`, `fuzzy-find` |
| **Project** | `build`, `test`, `deploy`, `git-commit` |

---

## 2. Resource Isolation & Sandbox Hardening

We must protect the **Coordination Kernel** from misbehaving WASM plugins.

### 2.1 Instruction Counting (CPU Fuel)
Using `wazero`'s instruction counting, we will wrap every plugin execution in a "Fuel" budget.

- **Mechanism**:
    1.  Maintain a `FuelBudget` in the `Instance`.
    2.  Use `wazero`'s listener to decrement fuel for every opcode.
    3.  If fuel reaches 0, the call is aborted with `ErrNotEnoughFuel`.
- **Policy**: Baseline fuel per `HandleMessage` call is 10M instructions (configurable).

### 2.2 Memory Hardening
- **Heap Limit**: Enforce `MaxMemory` per-plugin via `wazero.ModuleConfig.WithMaxMemory(pages)`.
- **Allocation Traps**: Catch `Out of Memory` errors in the plugin and transition its status to `StatusCrashed` rather than allowing it to hang.

### 2.3 Throttle System
Expand the existing message throttle to include:
- **Bytes-per-second**: Prevent "logging bombs" or huge binary transfers from saturating the kernel IPC.
- **Circuit Breaker**: Auto-disable a plugin if it crashes 3 times in 60 seconds.

---

## 3. Implementation Plan (DEVELOPER)

1.  **Refactor `pkg/frontend/modal`**:
    - Update `State` and `Driver`.
    - Implement the `Pending` key buffer logic in `Engine.Process`.
2.  **Harden `pkg/wasm/runtime`**:
    - Implement `WithMaxMemory`.
    - Add basic Instruction Counting listener (or use `context` timeouts as a proxy for the first iteration).
