# PLAN: Universal Modal Engine (Phase 6)

## 1. Philosophical Comparison

### Vim (Verb-Noun / Grammar)
- **Concept**: Language of editing. Keys are verbs (`d`, `y`, `c`) and nouns/motions (`w`, `ip`, `f`).
- **Philosophy**: Efficiency through composability. You describe the change you want to make.
- **State**: Modal (Normal, Insert, Visual). Actions usually follow the operator.

### Helix (Selection-Action)
- **Concept**: Object-Verb. You select the target first, then apply the action.
- **Philosophy**: Visual feedback and predictability. You always see what will be affected before you press the "verb".
- **State**: Modal, but selection is primary. No "Visual" mode separate from "Normal" because selection is always active.

### Emacs Meow (Position-Action)
- **Concept**: Modal editing for Emacs without head-on Vim emulation.
- **Philosophy**: Minimalist, staying close to hardware/physical layout. Uses "Keypad" mode for commands.
- **State**: Focuses on "Movement", "Selection", and "Command" transitions. Optimized for the physical keyboard layout rather than just mnemonics.

---

## 2. Implementation Strategy: `pkg/frontend/modal`

### 2.1 The Core Engine
We will create a provider-agnostic engine that maps raw key events to **Intents**.

- **`Engine`**: Interface that receives `KeyPress` and returns `Intent`.
- **`Intent`**: A high-level description of what the user wants to do (e.g., `Move(Down)`, `Delete(Line)`, `SwitchMode(Insert)`).
- **`Driver`**: Logic for specific philosophies (VimDriver, HelixDriver, MeowDriver).

### 2.2 Shared Interfaces (WIT-ready)
The engine will be usable by TUI, GUI, and Web frontends alike.

```go
type Intent interface{}

type MoveIntent struct { Direction string; Count int }
type ActionIntent struct { Verb string; Selection Selection }
type ModeIntent struct { NewMode string }
```

### 2.3 Integration Plan
1.  **Refactor `alloy-tui`**: Remove hardcoded key handling from `update.go`.
2.  **Create `VimDriver`**: Standardize the current TUI behavior.
3.  **Add `HelixDriver`**: Implement selection-first logic.
4.  **Expose to GUI**: Ensure the Gio frontend can pump raw keys into the same engine.

---

## 3. Next Steps (DEVELOPER)
1.  Initialize `pkg/frontend/modal` package.
2.  Port the existing TUI modal state constants to the new package.
3.  Define the `Driver` interface to support multiple philosophies.
