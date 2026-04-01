# UI Architecture: Modal Interaction & The Arbitrator Engine

Alloy frontends (TUI, GUI, Web) are built on a "Universal Modal Engine" that decouples keybinding logic and state transitions from the underlying UI rendering. This allows a consistent user experience regardless of the platform.

## 1. The Modal State Machine

Every frontend implements a three-tier modal state machine:

1.  **Selection/Normal Mode**: The default state for navigation, split management, and buffer switching.
2.  **Insert/Edit Mode**: Direct interaction with a specific buffer or textarea.
3.  **Command/Action Mode**: Transient states for Omni-palette search (`/`), command execution (`:`), or mnemonic trees (leaders).

## 2. Configurable Modal Drivers

Alloy allows users to swap the entire "interaction set" by selecting a Modal Driver. Each driver defines how the state machine transitions and what key chords perform common actions.

| Driver | Philosphy | Key Characteristics |
|--------|-----------|----------------------|
| **Neovim** | Modal / Positional | Traditional `h/j/k/l` navigation, `i` for insert, `:` for commands. |
| **Helix** | Selection -> Action | Object-verb interaction. Selections happen first, followed by modifiers. |
| **Meow** | Emacs Modal | Inspired by `emacs-meow`; combines structural movement with modal efficiency without the "Vim-heaviness". |

## 3. Implementation Plan: Cross-Frontend Parity

Currently, the TUI implements a prototype of the Neovim driver. The goal is to refactor this into a shared library.

### Phase 1: Core Modal Lib (`pkg/frontend/modal`)
- **Action Registry**: Map abstract actions (e.g., `BufferNext`, `PaneSplitVertical`, `SearchKnowledge`) to driver-specific key chords.
- **State Switcher**: A shared Go/WASM-ready component that manages `CurrentMode`.

### Phase 2: Driver Definitions
- [ ] **Neovim Driver**: Standardize Vim motions and commands.
- [ ] **Helix Driver**: Implement "Multiple Cursors" and selection-first logic.
- [ ] **Meow Driver**: Define structural movement maps.

### Phase 3: Frontend Adoption
- [ ] **Alloy TUI**: Refactor `cmd/alloy-tui/model.go` to use the shared modal library.
- [ ] **Alloy GUI (Gio)**: Implement the drawing of modal indicators and cursor styles in the hardware-accelerated view.
- [ ] **Alloy Web**: Hook the Go-WASM modal bridge into the JavaScript event listeners for sub-millisecond mode switching in the browser.
