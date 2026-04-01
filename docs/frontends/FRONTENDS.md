# Frontend Philosophy: The Workspace Composition Engine

Alloy's frontends (TUI, GUI, Web) are **Workspace Composition Engines**. They do not just "draw" pixels; they assemble a personal workspace by combining **Project Tools** with **User Tools** into a single project interface.

## 1. Defining the Workspace

In Alloy, the UI is a **Personal Synthesis** of three distinct layers of functionality:

### 1.1 Project Tools (Shared Effort)
Shared components used by everyone in the project (e.g., Team Chat, Project Dashboard, Activity Log). These are typically defined by a `Project Archetype` (Coding, Sales, etc.) and mandated by the project's manifest.

### 1.2 User Tools (Personal Capability)
User-specific components that are not visible to the team (e.g., Private Scratchpads, Personal TODO lists, or individual "side-car" plugins). The frontend is responsible for loading these from the user's local profile and integrating them into the active workspace layout.

### 1.3 Personalization (The Experience)
The user's preferred layout, theme, and navigation style (the **Modal Driver**).

---

## 2. Interaction & Modality
A workspace isn't just a layout; it is an **Interaction Set**. All frontends provide a **Unified Interaction Bridge** where all tools (both project and user) respect a shared modal state (Neovim, Helix, or Meow).

Alloy frontends provide a **Unified Interaction Bridge** where all components respect a shared modal state. This means you can use the same Neovim, Helix, or Meow navigation across different project views.

### 2.1 UI Arbitration
While the composition layer decides *what* is shown, the **Arbitrator Pattern** determines *how* its shown on the specific device.
- **TUI**: Renders as high-density text viewports with Bubbletea.
- **GUI (Gio)**: Renders with hardware acceleration, high-fidelity widgets, and smooth transitions.
- **Web**: Renders via a Go-WASM bridge and a modern CSS-driven layout.

## 2. Shared Interface Elements
The Frontend provides several standardized components that plugins can leverage without writing UI code.

- **The Smart Command Bar**: A global fuzzy search and mnemonic menu.
- **The Dashboard Grid**: A modular layout of status cards from active plugins.
- **Multipane Views**: The ability to split the screen into side-by-side plugin viewports.
- **Overlays**: Transient modals for quick input or confirmation.

## 3. Layout Control
The layout of these elements is controlled by the **Frontend Registry**, which merges two sources of truth:
1. **The Project Manifest (`.alloy/workspace.json`)**: Dictates which plugins are "pinned" or "tiled" for specific project workflows.
2. **The User Profile**: Allows the user to move, resize, or hide panes regardless of the project's defaults.

---

## 4. Current Implementations

All Alloy frontends, regardless of their technology stack (TUI, GUI, Web), share a common core integration layer provided by the backend and the frontend SDK.

### 1.1 Secure IPC Bridge
- **Transport**: Standard TCP or Unix Domain Sockets. 
- **Security**: Mandatory mTLS (mutual TLS). Every frontend must possess a valid certificate signed by the Alloy Machine CA to be allowed to route messages.
- **Protocol**: Single, bidirectional stream of newline-delimited JSON messages (`api.Message`).

### 1.2 Shared Frontend Client (`pkg/frontend`)
The `pkg/frontend` package provides a high-level Go SDK for building new frontends. It abstracts the complexities of mTLS handshaking, background message reading, and asynchronous event handling.

**Common Features:**
- **Identity Awareness**: Automatically handles the lookup of local machine identities and mTLS configuration.
- **Asynchronous Event Handling**: Provides an `OnMessage` hook to react to unsolicited events (e.g., system alerts, chat notifications) without active polling.
- **Request/Response Correlation**: Maps incoming responses back to their original requests using unique Message IDs.
- **Message History**: Maintains a local, non-persistent cache of recent messages for easy UI rendering.

### 1.3 Message Protocol
Frontends interact with the system by sending `api.Message` structures.
- **Targeting**: Frontends can address messages to the `kernel`, native internal plugins (e.g., `iam`), or WASM-based application plugins.
- **Capabilities**: Frontends typically start by sending a `discover` request to the `command-manager` to learn what actions are currently available in the running core instance.

---

## 2. Individual Frontends

### 2.1 Alloy TUI (Terminal User Interface)
- **Path**: `cmd/alloy-tui/`
- **Technology**: Go, [Bubbletea](https://github.com/charmbracelet/bubbletea), [Lipgloss](https://github.com/charmbracelet/lipgloss).
- **Core Dependencies**: Minimal. Only requires a standard TTY/Terminal.
- **Use Case**: Quick debugging, server-side management, and low-resource environments.

### 2.2 Alloy Desktop GUI (Stable)
- **Path**: `cmd/alloy-gui-gio/`
- **Technology**: Go, [Gio](https://gioui.org/).
- **Platform Focus**: Optimized for Linux Wayland, but cross-platform-capable via Gio.
- **Build Requirements**:
  Building the Gio GUI requires system-level development libraries for rendering and input handling (Vulkan, GLES, Wayland, Xkb).
- **Use Case**: Rich media interaction and high-performance hardware-accelerated rendering.

### 2.3 Alloy Wayland Native (Experimental)
- **Path**: `cmd/alloy-gui-wayland-native/`
- **Technology**: Pure Go, [go-wayland](https://github.com/rajveermalviya/go-wayland).
- **Status**: Experimental.
- **Pros**: **Zero C-dependencies** and no X11/Vulkan requirements.
- **Cons**: Software rendering (SHM) only; no hardware acceleration. Requires a Wayland compositor with `xdg-shell` support.
- **Build**: Uses the `experimental_wayland` build tag.

### 2.4 Alloy Web (Active)
- **Path**: `cmd/alloy-web/`
- **Technology**: Go (Native Host Proxy), Go-WASM (Internal Core), JavaScript/Tailwind (UI Bridge).
- **Architecture**: A three-tier model:
    1.  **Native Host Proxy**: A Go server that handles the mTLS/IPC connection to the Alloy Kernel and serves the web assets.
    2.  **Go-WASM Core**: The `pkg/frontend` logic compiled into the browser to provide sub-millisecond fuzzy search and state management.
    3.  **JavaScript Bridge**: A lightweight UI layer that renders the Omni-palette, dynamic forms, and notifications.
- **Development Dependencies**: 
    - **Go**: For building the WASM and Proxy binaries.
    - **Node.js / NPM**: For running UX/JS tests (`vitest`).
- **Setup**:
    ```bash
    # Install all JS-based testing and dev dependencies
    just install-web-deps
    
    # Run all web-specific tests (Go + JS)
    just test-web
    
    # Build and run a dedicated web instance
    just run-web --dedicated
    ```
- **Use Case**: Cross-platform access via any modern browser without local installation.

---

## 3. Developing a New Frontend

To create a new frontend for Alloy:
1. Import `github.com/james-nesbitt/alloy/pkg/frontend`.
2. Initialize a `frontend.NewClient` with a unique name and the path to the `alloy-core` socket.
3. Use `client.OnMessage` to update your UI state when events arrive.
4. Implement a message-sending loop using `client.Send`.
