//go:build tinygo || wasip1 || wasm
package wasm

import (
	"encoding/json"
	"fmt"
	"time"
	"unsafe"
)

// Message is a copy of the api.Message to avoid circular dependencies.
type Message struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Sender    string          `json:"sender"`
	Target    string          `json:"target,omitempty"`
	Method    string          `json:"method"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp int64           `json:"timestamp"`
	Actor     string          `json:"actor,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
}

// Capability describes a functionality provided by a component.
type Capability struct {
	Method      string            `json:"method,omitempty"`
	Description string            `json:"description,omitempty"`
	Shortcut    string            `json:"shortcut,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type OnMessageFunc func(msg Message) Message
type MessageHandler func(msg Message) Message
type OnInitFunc func() error
type OnStartFunc func()
type OnSaveFunc func() []byte
type OnLoadFunc func([]byte)

// Plugin represents the high-level Go SDK for Alloy WASM plugins.
type Plugin struct {
	id           string
	capabilities []Capability
	handlers     map[string]OnMessageFunc
	defaultHandler OnMessageFunc
	onInit       OnInitFunc
	onStart      OnStartFunc
	onSave       OnSaveFunc
	onLoad       OnLoadFunc

	// Common service clients
	Events *Events
	Chat   *ChatClient
	Iam    *IAMClient
	Projects *ProjectClient
}

// New creates a new plugin instance.
func New(id string) *Plugin {
	p := &Plugin{
		id:       id,
		handlers: make(map[string]OnMessageFunc),
	}
	p.Events = &Events{pluginID: id}
	p.Chat = &ChatClient{pluginID: id}
	p.Iam = &IAMClient{pluginID: id}
	p.Projects = &ProjectClient{pluginID: id}
	return p
}

// WithCapability adds a capability to the plugin.
func (p *Plugin) WithCapability(method, description, shortcut string) *Plugin {
	p.capabilities = append(p.capabilities, Capability{
		Method:      method,
		Description: description,
		Shortcut:    shortcut,
	})
	return p
}

// WithCapabilityAnnotations adds a capability with annotations (e.g., tags, types).
func (p *Plugin) WithCapabilityAnnotations(method, description string, annotations map[string]string) *Plugin {
	p.capabilities = append(p.capabilities, Capability{
		Method:      method,
		Description: description,
		Annotations: annotations,
	})
	return p
}

// Handle registers a message handler for a specific method.
func (p *Plugin) Handle(method string, fn OnMessageFunc) *Plugin {
	p.handlers[method] = fn
	return p
}

// Default registers a default message handler for unknown methods.
func (p *Plugin) Default(fn OnMessageFunc) *Plugin {
	p.defaultHandler = fn
	return p
}

// OnInit sets an initialization function to be called at startup.
func (p *Plugin) OnInit(fn OnInitFunc) *Plugin {
	p.onInit = fn
	return p
}

// OnStart sets a background process to be run after initialization.
func (p *Plugin) OnStart(fn OnStartFunc) *Plugin {
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

//go:wasmimport alloy get_next_message
func alloyGetNextMessage(ptr uint32, size uint32) uint32

//go:wasmimport alloy send_response
func alloySendResponse(ptr uint32, size uint32)

// Run registers the plugin with the host and enters a message pulling loop.
func (p *Plugin) Run() {
	// Register with the low-level SDK guest state
	SetCapabilities(p.capabilities)
	SetHandler(p.dispatch)

	// Start additional processes if provided.
	if p.onStart != nil {
		go p.onStart()
	}

	// Start the Pull Model Message Loop in a background goroutine.
	// This allows the plugin to receive messages even during initialization or 
	// long-running synchronous host calls.
	go func() {
		for {
			inPtr := uint32(uintptr(unsafe.Pointer(&inBuffer[0])))
			size := alloyGetNextMessage(inPtr, uint32(len(inBuffer)))
			if size == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			// Read and unmarshal the message from the input buffer
			var msg Message
			if err := json.Unmarshal(inBuffer[:size], &msg); err != nil {
				Log("Pull failed to unmarshal: " + err.Error())
				continue
			}

			// Process the message
			resp := p.dispatch(msg)
			if resp.ID == "" && resp.Target == "" {
				alloySendResponse(0, 0)
				continue
			}

			// Marshal and send the response
			data, err := json.Marshal(resp)
			if err != nil {
				Log("Pull failed to marshal resp: " + err.Error())
				alloySendResponse(0, 0)
				continue
			}

			// Use the output buffer for the response
			copy(outBuffer[:], data)
			alloySendResponse(uint32(uintptr(unsafe.Pointer(&outBuffer[0]))), uint32(len(data)))
		}
	}()

	// Run initialization
	if p.onInit != nil {
		if err := p.onInit(); err != nil {
			Log(fmt.Sprintf("Plugin init failed: %v", err))
			return
		}
	}

	// Signal to the host that we are ready
	PluginStarted()
	Log("Plugin " + p.id + " fully started and ready")

	// Stay alive by blocking main goroutine (prevents deadlock)
	SleepForever()
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
	if handler, ok := p.handlers[msg.Method]; ok {
		return handler(msg)
	}

	// Fallback to default handler
	if p.defaultHandler != nil {
		return p.defaultHandler(msg)
	}

	// Default response for unknown methods
	return Message{
		Type:    "response",
		Method:  msg.Method,
		Payload: json.RawMessage(`{"error":"method_not_found"}`),
	}
}

// Compatibility wrappers for existing plugins
func (p *Plugin) DefaultHandle(fn OnMessageFunc) *Plugin {
	return p.Default(fn)
}

func Reply(msg Message, payload any) Message {
	data, _ := json.Marshal(payload)
	return Message{
		ID: msg.ID + "-resp",
		Type: "response",
		Sender: msg.Target,
		Target: msg.Sender,
		Method: msg.Method,
		Payload: data,
	}
}

func ErrorReply(msg Message, err string) Message {
	return Message{
		ID: msg.ID + "-resp",
		Type: "response",
		Sender: msg.Target,
		Target: msg.Sender,
		Method: msg.Method,
		Payload: json.RawMessage(fmt.Sprintf(`{"error":%q}`, err)),
	}
}
