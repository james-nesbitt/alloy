# Alloy Frontends

Alloy is designed with a "headless-first" architecture. The core backend (`alloy-core`) is a long-running service that manages plugins and message routing, while frontends connect to it over a secure IPC bridge to provide user interfaces.

## 1. Common Frontend Architecture

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

### 2.4 Alloy Web (Future)
- **Technology**: Go (serving WASM), React/Vue.
- **Strategy**: Leverage the same `pkg/frontend` logic compiled to WASM to bridge browser-based JS events to the Alloy IPC socket (likely via a WebSocket proxy).

---

## 3. Developing a New Frontend

To create a new frontend for Alloy:
1. Import `github.com/james-nesbitt/alloy/pkg/frontend`.
2. Initialize a `frontend.NewClient` with a unique name and the path to the `alloy-core` socket.
3. Use `client.OnMessage` to update your UI state when events arrive.
4. Implement a message-sending loop using `client.Send`.
