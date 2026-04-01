# Phase 6: Advanced Refinement - Planning & Strategy

Phase 6 marks the transition from "core features" to "universal access and enterprise refinement." The following targets define the success of this phase.

## 1. Omni-Palette: The Unified Command Surface
The Omni-Palette (Shortcut: `Ctrl+P` or `Cmd+K`) is the primary navigation and action hub of Alloy.

### Goals:
- **Unified Results**: Aggregate results from `command-manager` (actions), `index` (knowledge), and `buffer` (files).
- **Fuzzy Matching**: Implement performant fuzzy searching in either a native Go service or a dedicated WASM plugin.
- **Context-Aware Recommendations**: Rank actions based on current user activity (e.g., if a buffer is open, show buffer-specific commands first).
- **Dynamic Icons & Metadata**: Show shortcuts, documentation snippets, and source icons for each result.

## 2. Universal Access: Alloy Web
Provide a high-fidelity experience in the browser without requiring a local installation.

### Goals:
- **WASM Bridge**: Adapt the `alloy-core` to compile to `js/wasm`, allowing the kernel to run entirely in the browser.
- **WebSocket/WebRTC Transport**: Add a new IPC transport layer for remote/hosted kernels.
- **PWA Integration**: Enable offline use and desktop integration for the browser client.

## 3. Enterprise Hardening: Resource Isolation
Standardize how plugins consume system resources to prevent "noisy neighbor" scenarios.

### Goals:
- **Memory Caps**: Assign hard memory limits (e.g., 64MB) to each plugin's `wazero` sandbox.
- **CPU Fuel**: Integrate `wazero`'s fuel system to preemptively stop infinite loops or heavy computation in guest modules.
- **Governance Policies**: Allow administrators (via `iam`) to define resource profiles for different categories of plugins.

## 4. UI/UX Refinement
- **Theming System**: Standardize color palettes and styling across TUI, GUI, and Web.
- **Integrated Terminal**: Embed a high-performance terminal emulator within the Alloy GUI.
- **Drag-and-Drop Layouts**: Allow users to rearrange dashboard widgets and editor panes via mouse interaction.
