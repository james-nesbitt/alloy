# TUI Design: Buffer & Command Management

This document outlines the design for the Alloy TUI (`alloy-tui`), focusing on a keyboard-centric, Vim/Emacs-inspired interface for managing buffers and interacting with plugins.

## Core Interface Principles

- **Modal Interaction**: Inspired by Vim, the TUI will have distinct modes (e.g., `Normal`, `Insert`, `Command-line`).
- **Command-Line First**: A dedicated command bar (minibuffer) for executing actions, searching, and switching buffers (e.g., `:buffer 1`, `:chat`, `:ai help`).
- **Pane Management**: Support for splitting the view into multiple panes (vertical/horizontal) to view different plugin contents simultaneously.
- **Mouse Support**: Click-to-focus panes and scroll support, integrated with `bubbletea`'s mouse events.
- **Dynamic Discovery**: Commands available in the command-line are dynamically populated from the `command-manager`.

## Layout Architecture

1.  **Header (Optional)**: Brief breadcrumbs or context (e.g., `Alloy > Buffer: main.go`).
2.  **Main Area (Dynamic Panes)**:
    -   Multiple `viewport` or `textarea` instances.
    -   Active pane highlighting.
3.  **Status Line**:
    -   **Left**: Current mode (NORMAL/INSERT/CMD), active user, and connection status.
    -   **Center**: Active buffer/plugin name.
    -   **Right**: Cursor position (row/col) and time.
4.  **Command Bar (Minibuffer)**:
    -   Hidden by default, activated by `:` (command) or `/` (search).
    -   Provides auto-completion for plugin methods discovered via `command-manager`.

## Keyboard Shortcuts (Vim-inspired)

| Key | Action |
|-----|--------|
| `i` | Enter Insert Mode (for textareas) |
| `Esc` | Return to Normal Mode |
| `:` | Open Command Bar |
| `Ctrl-w` + `h/j/k/l` | Navigate between panes |
| `Ctrl-w` + `v/s` | Split pane vertically/horizontally |
| `Ctrl-w` + `c` | Close current pane |
| `Tab` | (In Command Bar) Auto-complete command |

## Implementation Plan

### Phase 1: Enhanced TUI Foundation
- [ ] **State Machine**: Implement a robust mode-based state machine in `cmd/alloy-tui/main.go`.
- [ ] **Command Bar Component**: Create a dedicated component for the minibuffer with history and completion.
- [ ] **Layout Manager**: Implement a basic pane manager that can split the screen area between multiple `tea.Model` components.

### Phase 2: Buffer Service Integration
- [ ] **Buffer Listing**: Command `:ls` or `:buffers` calls `buffer:list`.
- [ ] **Buffer Selection**: Command `:b <id>` calls `buffer:open`.
- [ ] **Syncing events**: Handle `buffer:updated` events to refresh the active pane.

### Phase 3: Plugin Interaction
- [ ] **Generic Plugin Proxy**: Any method discovered via `command-manager` can be called via `:call <plugin> <method> <payload>`.
- [ ] **Specialized Views**: Create a "Chat View" for `chat` and a "Log View" for kernel logs.
