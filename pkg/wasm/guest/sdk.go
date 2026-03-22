//go:build wasip1 || wasm

package guest

import (
	"encoding/json"
	"fmt"

	guest "github.com/james-nesbitt/alloy/build/gen/bindings/guest"
)

// Message is a high-level representation of an Alloy message.
type Message struct {
	ID        string
	Type      string
	Sender    string
	Target    string
	Method    string
	Payload   []byte
	Timestamp int64
}

// Handler is a function that processes a message and returns an optional response.
type Handler func(msg Message) *Message

// AlloyHandler is a handler that uses the raw WIT message type.
type AlloyHandler func(msg guest.AlloyMessage) guest.AlloyMessage

// Command is a high-level representation of a plugin command.
type Command struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Handler     CommandHandler     `json:"-"`
	Shortcut    string            `json:"shortcut,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// CommandContext provides context for command execution.
type CommandContext struct {
	Plugin *Plugin
	Args   []string
	Sender string
}

// CommandHandler is a function that handles a command.
type CommandHandler func(ctx CommandContext) CommandResult

// CommandResult is the outcome of a command execution.
type CommandResult struct {
	Success bool            `json:"success"`
	Output  string          `json:"output"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// Plugin represents an Alloy WASM plugin with ergonomic Go bindings.
type Plugin struct {
	id            string
	capabilities  []guest.AlloyCapability
	handlers      map[string]Handler
	alloyHandlers map[string]AlloyHandler
	commands      map[string]Command
	onInit        func() error
	onStart       func()
	onShutdown    func()
}

// NewPlugin creates a new ergonomic Alloy plugin.
func NewPlugin(id string) *Plugin {
	return &Plugin{
		id:            id,
		handlers:      make(map[string]Handler),
		alloyHandlers: make(map[string]AlloyHandler),
		commands:      make(map[string]Command),
	}
}

// WithMetadata sets metadata for the plugin.
func (p *Plugin) WithMetadata(name, description, version, author string) *Plugin {
	return p
}

// WithTags adds tags to the plugin.
func (p *Plugin) WithTags(tags ...string) *Plugin {
	return p
}

// WithCapability adds a capability to the plugin.
func (p *Plugin) WithCapability(method, description string) *Plugin {
	p.capabilities = append(p.capabilities, guest.AlloyCapability{
		Method:      method,
		Description: description,
		Shortcut:    guest.None[string](),
		Annotations: guest.None[[]guest.AlloyTuple2StringStringT](),
	})
	return p
}

// RegisterMethod registers a handler for a specific message method.
func (p *Plugin) RegisterMethod(method string, description string, handler Handler) *Plugin {
	p.handlers[method] = handler
	return p.WithCapability(method, description)
}

// Handle registers a handler for a specific method (alias/compat for RegisterMethod).
func (p *Plugin) Handle(method string, handler any) *Plugin {
	if h, ok := handler.(Handler); ok {
		p.handlers[method] = h
	} else if ah, ok := handler.(AlloyHandler); ok {
		p.alloyHandlers[method] = ah
	} else if f, ok := handler.(func(guest.AlloyMessage) guest.AlloyMessage); ok {
		p.alloyHandlers[method] = AlloyHandler(f)
	}
	return p
}

// RegisterCommand registers a command with its handler.
func (p *Plugin) RegisterCommand(cmd Command) *Plugin {
	p.commands[cmd.Name] = cmd
	
	// Register the command as a capability with "command:" prefix
	method := "command:" + cmd.Name
	annots := make([]guest.AlloyTuple2StringStringT, 0, len(cmd.Annotations))
	for k, v := range cmd.Annotations {
		annots = append(annots, guest.AlloyTuple2StringStringT{F0: k, F1: v})
	}
	
	shortcut := guest.None[string]()
	if cmd.Shortcut != "" {
		shortcut = guest.Some(cmd.Shortcut)
	}

	p.capabilities = append(p.capabilities, guest.AlloyCapability{
		Method:      method,
		Description: cmd.Description,
		Shortcut:    shortcut,
		Annotations: guest.Some(annots),
	})
	return p
}

// OnInit sets the plugin's initialization function.
func (p *Plugin) OnInit(fn func() error) *Plugin {
	p.onInit = fn
	return p
}

// OnStart sets the plugin's start function.
func (p *Plugin) OnStart(fn func()) *Plugin {
	p.onStart = fn
	return p
}

// Run starts the plugin's message loop (alias for Serve).
func (p *Plugin) Run() error {
	p.Serve()
	return nil
}

// Serve starts the plugin's message loop.
func (p *Plugin) Serve() {
	// 1. Initialize with the host
	guest.AlloyInit(p.id, p.capabilities)

	// 2. Run user initialization
	if p.onInit != nil {
		if err := p.onInit(); err != nil {
			p.Log(LogLevelError, fmt.Sprintf("Initialization failed: %v", err))
			return
		}
	}

	// 3. Signal readiness
	guest.AlloyStarted()

	// 4. Run user onStart
	if p.onStart != nil {
		p.onStart()
	}

	// 5. Start message loop
	p.messageLoop()
}

func (p *Plugin) messageLoop() {
	for {
		optMsg := guest.AlloyGetNextMessage()
		if optMsg.IsNone() {
			continue
		}

		rawMsg := optMsg.Unwrap()
		
		// Priority 1: Raw AlloyHandlers
		if ah, ok := p.alloyHandlers[rawMsg.Method]; ok {
			resp := ah(rawMsg)
			if resp.Id != "" {
				guest.AlloySendResponse(resp)
			}
			continue
		}

		// Priority 2: SDK Handlers
		msg := Message{
			ID:      rawMsg.Id,
			Type:    rawMsg.MsgType,
			Method:  rawMsg.Method,
			Sender:  rawMsg.Sender,
			Payload: rawMsg.Payload,
		}

		var resp *Message

		// Check if it's a command
		if len(msg.Method) > 8 && msg.Method[:8] == "command:" {
			resp = p.handleCommand(msg)
		} else if handler, ok := p.handlers[msg.Method]; ok {
			resp = handler(msg)
		}

		if resp != nil {
			guest.AlloySendResponse(guest.AlloyMessage{
				Id:      resp.ID,
				MsgType: "response",
				Method:  resp.Method,
				Sender:  p.id,
				Target:  guest.Some(msg.Sender),
				Payload: resp.Payload,
			})
		}
	}
}

func (p *Plugin) handleCommand(msg Message) *Message {
	cmdName := msg.Method[8:]
	cmd, ok := p.commands[cmdName]
	if !ok {
		return p.ReplyError(msg, "Command not found")
	}

	var args []string
	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &args)
	}

	result := cmd.Handler(CommandContext{
		Plugin: p,
		Args:   args,
		Sender: msg.Sender,
	})

	payload, _ := json.Marshal(result)
	return &Message{
		ID:      msg.ID + "-resp",
		Method:  msg.Method,
		Payload: payload,
	}
}

// Log levels
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// Log logs a message to the host.
func (p *Plugin) Log(level string, msg string) {
	guest.AlloyLog(level, msg)
}

// RouteMessage sends a message to the host router.
func (p *Plugin) RouteMessage(msg guest.AlloyMessage) {
	guest.AlloyRouteMessage(msg)
}

// Call sends a message to the host and waits for a response.
func (p *Plugin) Call(msg guest.AlloyMessage) guest.AlloyMessage {
	return guest.AlloyCall(msg)
}

// Reply creates a response message for the given request.
func (p *Plugin) Reply(req guest.AlloyMessage, payload any) guest.AlloyMessage {
	data, _ := json.Marshal(payload)
	return guest.AlloyMessage{
		Id:      req.Id + "-resp",
		MsgType: "response",
		Method:  req.Method,
		Sender:  p.id,
		Target:  guest.Some(req.Sender),
		Payload: data,
	}
}

// ErrorReply creates an error response message.
func (p *Plugin) ErrorReply(req guest.AlloyMessage, errMsg string) guest.AlloyMessage {
	result := CommandResult{Success: false, Error: errMsg}
	data, _ := json.Marshal(result)
	return guest.AlloyMessage{
		Id:      req.Id + "-resp",
		MsgType: "response",
		Method:  req.Method,
		Sender:  p.id,
		Target:  guest.Some(req.Sender),
		Payload: data,
	}
}

// SDKReply creates a SDK response message for the given high-level request.
func (p *Plugin) SDKReply(req Message, method string, payload any) *Message {
	data, _ := json.Marshal(payload)
	return &Message{
		ID:      req.ID + "-resp",
		Type:    "response",
		Method:  method,
		Target:  req.Sender,
		Payload: data,
	}
}

// ReplyError creates an error response message.
func (p *Plugin) ReplyError(req Message, errMsg string) *Message {
	result := CommandResult{Success: false, Error: errMsg}
	data, _ := json.Marshal(result)
	return &Message{
		ID:      req.ID + "-resp",
		Type:    "response",
		Method:  req.Method,
		Target:  req.Sender,
		Payload: data,
	}
}

// KV Utils
func (p *Plugin) KVSet(key string, val []byte) bool {
	return guest.AlloyKvSet(key, val)
}

func (p *Plugin) KVGet(key string) ([]byte, bool) {
	res := guest.AlloyKvGet(key)
	if res.IsSome() {
		return res.Unwrap(), true
	}
	return nil, false
}

func (p *Plugin) KVDelete(key string) bool {
	return guest.AlloyKvDelete(key)
}

func (p *Plugin) KVList(prefix string) []string {
	return guest.AlloyKvList(prefix)
}
