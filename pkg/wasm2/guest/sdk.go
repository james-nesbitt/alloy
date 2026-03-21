package guest

import (
	"encoding/json"
	"./wit/guest"
)

// Plugin represents a WASM plugin using WIT bindings.
type Plugin struct {
	id               string
	capabilities     []guest.AlloyCapability
	handlers         map[string]MessageHandler
	commandHandlers  map[string]CommandHandler
	defaultHandler   MessageHandler
	onInit           func() error
	onStart          func()
	onSave           func() []byte
	onLoad           func([]byte)
	witInstance      *guest.AlloyInstance
	metadata         PluginMetadata
}

// PluginMetadata contains discoverable information about a plugin.
type PluginMetadata struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Version      string            `json:"version"`
	Author       string            `json:"author"`
	Tags         []string          `json:"tags"`
	Capabilities []CapabilityInfo  `json:"capabilities"`
}

// CapabilityInfo describes a discoverable capability.
type CapabilityInfo struct {
	Method       string            `json:"method"`
	Description  string            `json:"description"`
	Shortcut     string            `json:"shortcut,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Type         string            `json:"type,omitempty"` // "message" or "command"
}

// MessageHandler handles incoming messages.
type MessageHandler func(msg guest.AlloyMessage) guest.AlloyMessage

// CommandHandler handles command execution.
type CommandHandler func(cmd Command) CommandResult

// Command represents an executable command.
type Command struct {
	Name    string          `json:"name"`
	Args    []string        `json:"args"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// CommandResult represents the result of command execution.
type CommandResult struct {
	Success bool            `json:"success"`
	Output  string          `json:"output"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// NewPlugin creates a new WIT-based plugin.
func NewPlugin(id string) *Plugin {
	return &Plugin{
		id:              id,
		handlers:        make(map[string]MessageHandler),
		commandHandlers: make(map[string]CommandHandler),
		metadata: PluginMetadata{
			Name:        id,
			Capabilities: []CapabilityInfo{},
		},
	}
}

// WithMetadata sets the plugin metadata.
func (p *Plugin) WithMetadata(name, description, version, author string) *Plugin {
	p.metadata.Name = name
	p.metadata.Description = description
	p.metadata.Version = version
	p.metadata.Author = author
	return p
}

// WithTags adds tags to the plugin.
func (p *Plugin) WithTags(tags ...string) *Plugin {
	p.metadata.Tags = append(p.metadata.Tags, tags...)
	return p
}

// WithCapability adds a capability to the plugin.
func (p *Plugin) WithCapability(method, description string) *Plugin {
	p.capabilities = append(p.capabilities, guest.AlloyCapability{
		Method:      method,
		Description: description,
	})

	// Add to discoverable capabilities
	p.metadata.Capabilities = append(p.metadata.Capabilities, CapabilityInfo{
		Method:      method,
		Description: description,
		Type:        "message",
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

	// Add to discoverable capabilities
	capInfo := CapabilityInfo{
		Method:      method,
		Description: description,
		Annotations: annotations,
		Type:        "message",
	}
	p.metadata.Capabilities = append(p.metadata.Capabilities, capInfo)

	return p
}

// WithCommand adds a command to the plugin.
func (p *Plugin) WithCommand(name, description string) *Plugin {
	// Register the command capability
	capMethod := "command:" + name
	p.capabilities = append(p.capabilities, guest.AlloyCapability{
		Method:      capMethod,
		Description: description,
	})

	// Add to discoverable capabilities
	p.metadata.Capabilities = append(p.metadata.Capabilities, CapabilityInfo{
		Method:      name,
		Description: description,
		Type:        "command",
	})

	return p
}

// Handle registers a message handler for a specific method.
func (p *Plugin) Handle(method string, handler MessageHandler) *Plugin {
	p.handlers[method] = handler
	return p
}

// HandleCommand registers a command handler.
func (p *Plugin) HandleCommand(name string, handler CommandHandler) *Plugin {
	p.commandHandlers[name] = handler
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

// GetMetadata returns the plugin metadata.
func (p *Plugin) GetMetadata() PluginMetadata {
	return p.metadata
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

	// Register metadata with the host
	metadataJSON, err := json.Marshal(p.metadata)
	if err == nil {
		instance.AlloyKvSet("plugin:metadata:"+p.id, metadataJSON)
	}

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
			// Check if this is a command
			if p.isCommandMessage(msg) {
				resp = p.handleCommandMessage(msg)
			} else {
				// Check for a specific message handler
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
		}

		// Send the response if we have one
		if resp.Id != "" {
			p.witInstance.AlloySendResponse(resp)
		}
	}
}

// isCommandMessage checks if a message represents a command.
func (p *Plugin) isCommandMessage(msg guest.AlloyMessage) bool {
	// Check if this is a command message
	if len(msg.Method) > 8 && msg.Method[:8] == "command:" {
		return true
	}
	return false
}

// handleCommandMessage handles a command message.
func (p *Plugin) handleCommandMessage(msg guest.AlloyMessage) guest.AlloyMessage {
	// Extract command name
	commandName := msg.Method[8:] // Remove "command:" prefix

	// Parse the command
	var cmd Command
	if err := json.Unmarshal(msg.Payload, &cmd); err != nil {
		return ErrorReply(msg, "invalid_command_payload")
	}

	// Set command name if not provided in payload
	if cmd.Name == "" {
		cmd.Name = commandName
	}

	// Find the command handler
	handler, ok := p.commandHandlers[cmd.Name]
	if !ok {
		return ErrorReply(msg, "command_not_found")
	}

	// Execute the command
	result := handler(cmd)

	// Create response
	respData, err := json.Marshal(result)
	if err != nil {
		return ErrorReply(msg, "failed_to_marshal_result")
	}

	return guest.AlloyMessage{
		Id:      msg.Id + "-response",
		Method:  msg.Method,
		Sender:  p.id,
		Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
		Payload: respData,
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

// ExecuteCommand executes a command on another plugin.
func (p *Plugin) ExecuteCommand(targetPlugin, commandName string, cmd Command) (CommandResult, bool) {
	// Create a command message
	cmd.Method = "command:" + commandName
	cmdData, err := json.Marshal(cmd)
	if err != nil {
		return CommandResult{Success: false, Error: "failed_to_marshal_command"}, false
	}

	// Create the message
	msg := guest.AlloyMessage{
		Method:  "command:" + commandName,
		Sender:  p.id,
		Target:  guest.AlloyOption[string]{Value: targetPlugin, Set: true},
		Payload: cmdData,
	}

	// Call the target plugin
	resp, ok := p.Call(msg)
	if !ok {
		return CommandResult{Success: false, Error: "command_execution_failed"}, false
	}

	// Parse the response
	var result CommandResult
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		return CommandResult{Success: false, Error: "failed_to_parse_result"}, false
	}

	return result, true
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

// SuccessCommand creates a successful command result.
func SuccessCommand(output string, data any) CommandResult {
	var dataJSON json.RawMessage
	if data != nil {
		dataBytes, err := json.Marshal(data)
		if err == nil {
			dataJSON = dataBytes
		}
	}

	return CommandResult{
		Success: true,
		Output:  output,
		Data:    dataJSON,
	}
}

// ErrorCommand creates an error command result.
func ErrorCommand(output, errorMsg string) CommandResult {
	return CommandResult{
		Success: false,
		Output:  output,
		Error:   errorMsg,
	}
}