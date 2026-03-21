package guest

import (
	"encoding/json"
	"github.com/jnesbitt/alloy-go/pkg/wasm2/bindings/guest"
)

// Plugin represents a WASM plugin using WIT bindings.	ype Plugin struct {
	id           string
	capabilities []guest.AlloyCapability
	handlers     map[string]MessageHandler
	defaultHandler MessageHandler
	onInit       func() error
	onStart      func()
	onSave       func() []byte
	onLoad       func([]byte)
	witInstance  *guest.AlloyInstance
}

// MessageHandler handles incoming messages.
type MessageHandler func(msg guest.AlloyMessage) guest.AlloyMessage

// NewPlugin creates a new WIT-based plugin.
func NewPlugin(id string) *Plugin {
	return &Plugin{
		id:       id,
		handlers: make(map[string]MessageHandler),
	}
}

// WithCapability adds a capability to the plugin.unc (p *Plugin) WithCapability(method, description string) *Plugin {
	p.capabilities = append(p.capabilities, guest.AlloyCapability{
		Method:      method,
		Description: description,
	})
	return p
}

// WithCapabilityAnnotations adds a capability with annotations.
func (p *Plugin) WithCapabilityAnnotations(method, description string, annotations map[string]string) *Plugin {
	cap := guest.AlloyCapability{
		Method:      method,
		Description: description,
	}

	if len(annotations) > 0 {
		annotList := make([]guest.AlloyTuple2StringStringT, 0, len(annotations))
		for k, v := range annotations {
			annotList = append(annotList, guest.AlloyTuple2StringStringT{F0: k, F1: v})
		}
		cap.Annotations = guest.AlloyOption[[]guest.AlloyTuple2StringStringT]{
			Value: annotList,
			Set:   true,
		}
	}

	p.capabilities = append(p.capabilities, cap)
	return p
}

// Handle registers a message handler for a specific method.
func (p *Plugin) Handle(method string, handler MessageHandler) *Plugin {
	p.handlers[method] = handler
	return p
}

// Default registers a default message handler for unknown methods.
func (p *Plugin) Default(handler MessageHandler) *Plugin {
	p.defaultHandler = handler
	return p
}

// OnInit sets an initialization function.
func (p *Plugin) OnInit(fn func() error) *Plugin {
	p.onInit = fn
	return p
}

// OnStart sets a background process to run after initialization.
func (p *Plugin) OnStart(fn func()) *Plugin {
	p.onStart = fn
	return p
}

// OnSave sets a function to save plugin state.
func (p *Plugin) OnSave(fn func() []byte) *Plugin {
	p.onSave = fn
	p.WithCapability("system:save_state", "Internal: Save state for reload")
	return p
}

// OnLoad sets a function to load plugin state.
func (p *Plugin) OnLoad(fn func([]byte)) *Plugin {
	p.onLoad = fn
	p.WithCapability("system:load_state", "Internal: Load state for reload")
	return p
}

// Run starts the plugin and enters the message loop.
func (p *Plugin) Run() error {
	// Create the WIT instance
	instance, err := guest.NewAlloyInstance(nil)
	if err != nil {
		return err
	}
	p.witInstance = instance

	// Initialize the plugin
	instance.AlloyInit(p.id, p.capabilities)

	// Run initialization if provided
	if p.onInit != nil {
		if err := p.onInit(); err != nil {
			return err
		}
	}

	// Start background process if provided
	if p.onStart != nil {
		go p.onStart()
	}

	// Signal that the plugin is ready
	instance.AlloyStarted()

	// Enter the message loop
	return p.messageLoop()
}

// messageLoop handles incoming messages.
func (p *Plugin) messageLoop() error {
	for {
		// Get the next message
		optMsg := p.witInstance.AlloyGetNextMessage()
		if !optMsg.Set {
			// No message available, yield
			p.witInstance.AlloyYield()
			continue
		}

		msg := optMsg.Value
		var resp guest.AlloyMessage

		// Handle system messages
		switch msg.Method {
		case "system:save_state":
			if p.onSave != nil {
				resp = guest.AlloyMessage{
					Id:      msg.Id + "-response",
					Method:  msg.Method,
					Sender:  p.id,
					Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
					Payload: p.onSave(),
				}
			}
		case "system:load_state":
			if p.onLoad != nil && len(msg.Payload) > 0 {
				p.onLoad(msg.Payload)
				resp = guest.AlloyMessage{
					Id:      msg.Id + "-response",
					Method:  msg.Method,
					Sender:  p.id,
					Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
				}
			}
		default:
			// Check for a specific handler
			handler, ok := p.handlers[msg.Method]
			if ok {
				resp = handler(msg)
			} else if p.defaultHandler != nil {
				resp = p.defaultHandler(msg)
			} else {
				// No handler found
				errMsg := map[string]string{"error": "method_not_found"}
				errData, _ := json.Marshal(errMsg)
				resp = guest.AlloyMessage{
					Id:      msg.Id + "-response",
					Method:  msg.Method,
					Sender:  p.id,
					Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
					Payload: errData,
				}
			}
		}

		// Send the response if we have one
		if resp.Id != "" {
			p.witInstance.AlloySendResponse(resp)
		}
	}
}

// Log logs a message to the host.
func (p *Plugin) Log(level, message string) {
	p.witInstance.AlloyLog(level, message)
}

// KVSet sets a key-value pair in storage.
func (p *Plugin) KVSet(key string, value []byte) bool {
	return p.witInstance.AlloyKvSet(key, value)
}

// KVGet gets a value from storage.
func (p *Plugin) KVGet(key string) ([]byte, bool) {
	optVal := p.witInstance.AlloyKvGet(key)
	if optVal.Set {
		return optVal.Value, true
	}
	return nil, false
}

// KVDelete deletes a key from storage.
func (p *Plugin) KVDelete(key string) bool {
	return p.witInstance.AlloyKvDelete(key)
}

// KVList lists keys with a prefix.
func (p *Plugin) KVList(prefix string) ([]string, bool) {
	return p.witInstance.AlloyKvList(prefix)
}

// RouteMessage routes a message to its target.
func (p *Plugin) RouteMessage(msg guest.AlloyMessage) {
	p.witInstance.AlloyRouteMessage(msg)
}

// Call performs a synchronous call to another plugin.
func (p *Plugin) Call(msg guest.AlloyMessage) (guest.AlloyMessage, bool) {
	return p.witInstance.AlloyCall(msg)
}

// Reply creates a response message.
func Reply(msg guest.AlloyMessage, payload any) guest.AlloyMessage {
	var payloadData []byte
	if payload != nil {
		var err error
		payloadData, err = json.Marshal(payload)
		if err != nil {
			payloadData = []byte(`{"error":"failed_to_marshal_payload"}`)
		}
	}

	return guest.AlloyMessage{
		Id:      msg.Id + "-response",
		Method:  msg.Method,
		Sender:  msg.Target.Value, // Response comes from the target
		Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
		Payload: payloadData,
	}
}

// ErrorReply creates an error response message.
func ErrorReply(msg guest.AlloyMessage, err string) guest.AlloyMessage {
	errMsg := map[string]string{"error": err}
	errData, _ := json.Marshal(errMsg)

	return guest.AlloyMessage{
		Id:      msg.Id + "-response",
		Method:  msg.Method,
		Sender:  msg.Target.Value, // Response comes from the target
		Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
		Payload: errData,
	}
}