package wasm

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Plugin represents the high-level framework for an Alloy WASM plugin.
type Plugin struct {
	id           string
	capabilities []Capability
	handlers     map[string]MessageHandler
	defaultHandler MessageHandler
	onInit       func() error
	onStart      func()
	onSave       func() []byte
	onLoad       func([]byte)

	// High-level clients
	Events   *Events
	Projects *ProjectClient
	Chat     *ChatClient
	IAM      *IAMClient
}

// New creates a new Plugin instance.
func New(id string) *Plugin {
	return &Plugin{
		id:       id,
		handlers: make(map[string]MessageHandler),
		Events:   NewEvents(id),
		Projects: NewProjectClient(id),
		Chat:     NewChatClient(id),
		IAM:      NewIAMClient(id),
	}
}

// WithCapability registers a plugin capability.
func (p *Plugin) WithCapability(method, description, shortcut string) *Plugin {
	p.capabilities = append(p.capabilities, Capability{
		Method:      method,
		Description: description,
		Shortcut:    shortcut,
	})
	return p
}

// Handle registers a handler function for a specific method.
func (p *Plugin) Handle(method string, handler MessageHandler) *Plugin {
	p.handlers[method] = handler
	return p
}

// DefaultHandle registers a fallback handler for methods without a specific handler.
func (p *Plugin) DefaultHandle(handler MessageHandler) *Plugin {
	p.defaultHandler = handler
	return p
}

// OnInit sets the initialization function.
func (p *Plugin) OnInit(fn func() error) *Plugin {
	p.onInit = fn
	return p
}

// OnStart sets the function to run when the plugin starts (usually for long-running loops).
func (p *Plugin) OnStart(fn func()) *Plugin {
	p.onStart = fn
	return p
}

// OnSave sets a function to be called when the plugin needs to save its internal state.
func (p *Plugin) OnSave(fn func() []byte) *Plugin {
	p.onSave = fn
	p.WithCapability("system:save_state", "Internal: Save state for reload", "")
	return p
}

// OnLoad sets a function to be called when the plugin needs to restore its internal state.
func (p *Plugin) OnLoad(fn func([]byte)) *Plugin {
	p.onLoad = fn
	p.WithCapability("system:load_state", "Internal: Load state for reload", "")
	return p
}

// Run registers the plugin with the host and enters a stay-alive loop.
func (p *Plugin) Run() {
	// Register with the low-level SDK guest state
	SetCapabilities(p.capabilities)
	SetHandler(p.dispatch)

	// Run initialization
	if p.onInit != nil {
		if err := p.onInit(); err != nil {
			Log(fmt.Sprintf("Plugin init failed: %v", err))
			return
		}
	}

	// Start main logic if provided.
	if p.onStart != nil {
		go p.onStart()
	}

	// Signal to the host that we are ready
	PluginStarted()

	// Stay alive without blocking host calls.
	// For Standard Go WASM, we must keep the main goroutine alive.
	// select{} is fine but we keep the Sleep loop for better cross-runtime behavior.
	for {
		time.Sleep(time.Hour)
	}
}

// dispatch routes messages to registered handlers.
func (p *Plugin) dispatch(msg Message) Message {
	if msg.Method == "system:save_state" && p.onSave != nil {
		return Message{Type: "response", Method: msg.Method, Payload: p.onSave()}
	}

	if msg.Method == "system:load_state" && p.onLoad != nil {
		p.onLoad(msg.Payload)
		return Message{Type: "response", Method: msg.Method}
	}

	// Check for a specific handler first
	if h, ok := p.handlers[msg.Method]; ok {
		return h(msg)
	}

	// Use default handler if available
	if p.defaultHandler != nil {
		return p.defaultHandler(msg)
	}

	// Check if this is a "ping" which is often handled by core
	if msg.Method == "ping" {
		return Message{
			Type:    "response",
			Method:  "pong",
			Payload: json.RawMessage(`{"status":"ok"}`),
		}
	}

	// Special case: automated help for discovered capabilities
	if msg.Method == "help" {
		var help strings.Builder
		help.WriteString(fmt.Sprintf("Plugin: %s\nAvailable methods:\n", p.id))
		for _, cap := range p.capabilities {
			help.WriteString(fmt.Sprintf("- %s: %s (shortcut: %s)\n", cap.Method, cap.Description, cap.Shortcut))
		}
		return Message{
			Type:    "response",
			Method:  "help",
			Payload: json.RawMessage(fmt.Sprintf("%q", help.String())),
		}
	}

	// Return an error if no handler found
	errPayload, _ := json.Marshal(map[string]string{"error": "method_not_found", "method": msg.Method})
	return Message{
		Type:    "error",
		Method:  msg.Method,
		Payload: json.RawMessage(errPayload),
	}
}

// Reply is a helper to create a response message.
func Reply(msg Message, payload interface{}) Message {
	data, _ := json.Marshal(payload)
	return Message{
		ID:      msg.ID,
		Type:    "response",
		Sender:  msg.Target, // We are now the sender
		Target:  msg.Sender, // Original sender is the target
		Method:  msg.Method,
		Payload: data,
	}
}

// ErrorReply is a helper to create an error message.
func ErrorReply(msg Message, err string) Message {
	data, _ := json.Marshal(map[string]string{"error": err})
	return Message{
		ID:      msg.ID,
		Type:    "error",
		Sender:  msg.Target,
		Target:  msg.Sender,
		Method:  msg.Method,
		Payload: data,
	}
}
