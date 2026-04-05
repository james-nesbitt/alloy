package guest

import (
	"encoding/json"
	"fmt"
	"unsafe"
)

// Plugin represents an Alloy WASM plugin with ergonomic Go bindings.
type Plugin struct {
	id            string
	capabilities  []AlloyCapability
	handlers      map[string]Handler
	alloyHandlers map[string]AlloyHandler
	commands      map[string]Command
	onInit        func() error
	onStart       func()
	onShutdown    func()
	background    bool           // Phase 10
	host          HostInterface
}

// NewPlugin creates a new ergonomic Alloy plugin.
func NewPlugin(id string) *Plugin {
	p := &Plugin{
		id:            id,
		handlers:      make(map[string]Handler),
		alloyHandlers: make(map[string]AlloyHandler),
		commands:      make(map[string]Command),
		background:    false,
	}
	p.host = createDefaultHost()
	return p
}
// ID returns the plugin's ID.
func (p *Plugin) ID() string {
	return p.id
}



// SetBackground sets whether this plugin runs in the background (Phase 10).
func (p *Plugin) SetBackground(bg bool) *Plugin {
	p.background = bg
	return p
}

// SetHost manually overrides the host interface. Useful for tests.
func (p *Plugin) SetHost(h HostInterface) {
	p.host = h
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
	p.capabilities = append(p.capabilities, AlloyCapability{
		Method:      method,
		Description: description,
		Shortcut:    None[string](),
		Annotations: None[[]AlloyTuple2StringStringT](),
		Intents:     None[[]string](),
	})
	return p
}

// WithIntent adds an intent to the last added capability (Phase 10).
func (p *Plugin) WithIntent(intent string) *Plugin {
	if len(p.capabilities) > 0 {
		var intents []string
		if p.capabilities[len(p.capabilities)-1].Intents.IsSome() {
			intents = p.capabilities[len(p.capabilities)-1].Intents.Unwrap()
		}
		intents = append(intents, intent)
		p.capabilities[len(p.capabilities)-1].Intents = Some(intents)
	}
	return p
}

// WithShortcut adds a shortcut to the last added capability.
func (p *Plugin) WithShortcut(shortcut string) *Plugin {
	if len(p.capabilities) > 0 {
		p.capabilities[len(p.capabilities)-1].Shortcut = Some(shortcut)
	}
	return p
}

// WithAnnotations adds annotations to the last added capability.
func (p *Plugin) WithAnnotations(method string, annotations map[string]string) *Plugin {
	for i, cap := range p.capabilities {
		if cap.Method == method {
			annots := make([]AlloyTuple2StringStringT, 0, len(annotations))
			for k, v := range annotations {
				annots = append(annots, AlloyTuple2StringStringT{F0: k, F1: v})
			}
			p.capabilities[i].Annotations = Some(annots)
			break
		}
	}
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
	} else if f, ok := handler.(func(AlloyMessage) AlloyMessage); ok {
		p.alloyHandlers[method] = AlloyHandler(f)
	}
	return p
}

// RegisterCommand registers a command with its handler.
func (p *Plugin) RegisterCommand(cmd Command) *Plugin {
	p.commands[cmd.Name] = cmd

	// Register the command as a capability with "command:" prefix
	method := "command:" + cmd.Name
	annots := make([]AlloyTuple2StringStringT, 0, len(cmd.Annotations))
	for k, v := range cmd.Annotations {
		annots = append(annots, AlloyTuple2StringStringT{F0: k, F1: v})
	}

	shortcut := None[string]()
	if cmd.Shortcut != "" {
		shortcut = Some(cmd.Shortcut)
	}

	p.capabilities = append(p.capabilities, AlloyCapability{
		Method:      method,
		Description: cmd.Description,
		Shortcut:    shortcut,
		Annotations: Some(annots),
		Intents:     None[[]string](),
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
	p.host.Init(p.id, p.capabilities, p.background)

	// 2. Run user initialization
	if p.onInit != nil {
		if err := p.onInit(); err != nil {
			p.Log(LogLevelError, fmt.Sprintf("Initialization failed: %v", err))
			return
		}
	}

	// 3. Signal readiness
	p.host.Started()

	// 4. Run user onStart
	if p.onStart != nil {
		p.onStart()
	}

	// 5. Start message loop
	p.messageLoop()
}

func (p *Plugin) messageLoop() {
	for {
		optMsg := p.host.GetNextMessage()
		if optMsg.IsNone() {
			continue
		}

		msg := optMsg.Unwrap()

		// Priority 1: AlloyHandlers
		if ah, ok := p.alloyHandlers[msg.Method]; ok {
			resp := ah(msg)
			if resp.Id != "" {
				p.host.SendResponse(resp)
			}
			continue
		}

		var resp *AlloyMessage

		// Check if it's a command
		if len(msg.Method) > 8 && msg.Method[:8] == "command:" {
			resp = p.handleCommand(msg)
		} else if handler, ok := p.handlers[msg.Method]; ok {
			resp = handler(msg)
		}

		if resp != nil {
			p.host.SendResponse(AlloyMessage{
				Id:        resp.Id,
				MsgType:   "response",
				Method:    resp.Method,
				Sender:    p.id,
				Actor:     resp.Actor,
				Target:    Some(msg.Sender),
				Payload:   resp.Payload,
				Timestamp: resp.Timestamp,
				Metadata:  resp.Metadata,
			})
		}
	}
}

func (p *Plugin) handleCommand(msg AlloyMessage) *AlloyMessage {
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
		Actor:  msg.Actor,
	})

	payload, _ := json.Marshal(result)
	return &AlloyMessage{
		Id:      msg.Id + "-resp",
		Method:  msg.Method,
		Payload: payload,
	}
}

// GetMetadata retrieves a metadata value by key.
func (m *AlloyMessage) GetMetadata(key string) (string, bool) {
	if m.Metadata == nil {
		return "", false
	}
	for _, entry := range m.Metadata {
		if entry.F0 == key {
			return entry.F1, true
		}
	}
	return "", false
}

// ContextID returns the context/namespace ID if present in metadata.
func (m *AlloyMessage) ContextID() string {
	val, ok := m.GetMetadata("context")
	if ok {
		return val
	}
	val, ok = m.GetMetadata("namespace")
	if ok {
		return val
	}
	return ""
}

// DispatchIntent sends an intent to the host to be routed (Phase 10).
func (p *Plugin) DispatchIntent(name string, payload any, contextID string) {
	data, _ := json.Marshal(payload)
	intent := AlloyIntent{
		Id:      fmt.Sprintf("%s-%d", name, p.Timestamp()),
		Name:    name,
		Sender:  p.id,
		Payload: data,
	}
	if contextID != "" {
		intent.ContextID = Some(contextID)
	} else {
		intent.ContextID = None[string]()
	}
	p.host.DispatchIntent(intent)
}

// DispatchVisualIntent sends a visual intent overlay for a shared buffer (Phase 12).
func (p *Plugin) DispatchVisualIntent(bufferID string, intentType string, offset, length uint32, color, label string) {
	p.host.DispatchVisualIntent(AlloyVisualIntent{
		BufferID:   bufferID,
		IntentType: intentType,
		Offset:     offset,
		Length:     length,
		Color:      color,
		Label:      label,
	})
}

// ProposeIntent suggests an intent to the user or kernel (Phase 12).
func (p *Plugin) ProposeIntent(name string, description string, payload any, contextID string) {
	data, _ := json.Marshal(payload)
	ctxID := None[string]()
	if contextID != "" {
		ctxID = Some(contextID)
	}
	p.host.ProposeIntent(AlloyProposeIntent{
		Id:          fmt.Sprintf("propose-%s-%d", name, p.Timestamp()),
		Name:        name,
		Description: description,
		Payload:     data,
		ContextID:   ctxID,
	})
}

// Timestamp returns a current timestamp in milliseconds.

func (p *Plugin) Timestamp() uint64 {
	// Best-effort timestamp from host or local (if available)
	return 0 // TODO: Implement robust timestamp in SDK
}

// Log logs a message to the host.
func (p *Plugin) Log(level string, msg string) {
	p.host.Log(level, msg)
}

// RouteMessage sends a message to the host router.
func (p *Plugin) RouteMessage(msg AlloyMessage) {
	p.host.RouteMessage(msg)
}

// Call sends a message to the host and waits for a response.
func (p *Plugin) Call(msg AlloyMessage) AlloyMessage {
	return p.host.Call(msg)
}

// Reply creates a response message for the given request.
func (p *Plugin) Reply(req AlloyMessage, payload any) AlloyMessage {
	data, _ := json.Marshal(payload)
	return AlloyMessage{
		Id:        req.Id + "-resp",
		MsgType:   "response",
		Method:    req.Method,
		Sender:    p.id,
		Actor:     req.Actor,
		Target:    Some(req.Sender),
		Payload:   data,
		Timestamp: req.Timestamp,
		Metadata:  req.Metadata,
	}
}

// ErrorReply creates an error response message.
func (p *Plugin) ErrorReply(req AlloyMessage, errMsg string) AlloyMessage {
	result := CommandResult{Success: false, Error: errMsg}
	data, _ := json.Marshal(result)
	return AlloyMessage{
		Id:        req.Id + "-resp",
		MsgType:   "response",
		Method:    "error",
		Sender:    p.id,
		Actor:     req.Actor,
		Target:    Some(req.Sender),
		Payload:   data,
		Timestamp: req.Timestamp,
		Metadata:  req.Metadata,
	}
}

// ReplyError creates an error response message (compatible with high-level msg types).
func (p *Plugin) ReplyError(req AlloyMessage, errMsg string) *AlloyMessage {
	result := CommandResult{Success: false, Error: errMsg}
	data, _ := json.Marshal(result)
	return &AlloyMessage{
		Id:      req.Id + "-resp",
		Method:  "error",
		Payload: data,
	}
}

// RequireService checks if a specific service is available.
func (p *Plugin) RequireService(serviceID string) {
	providers := p.FindProviders("*", "", "")
	found := false
	for _, provider := range providers {
		if provider == serviceID {
			found = true
			break
		}
	}
	if !found {
		p.Log(LogLevelError, fmt.Sprintf("Required service '%s' not found. Plugin exiting.", serviceID))
		panic(fmt.Sprintf("missing required service: %s", serviceID))
	}
}

// CheckCapability checks if any plugin provides the specified method.
func (p *Plugin) CheckCapability(method string) bool {
	providers := p.FindProviders(method, "", "")
	return len(providers) > 0
}

// RequireCapability checks if any plugin provides the specified method.
func (p *Plugin) RequireCapability(method string) {
	if !p.CheckCapability(method) {
		p.Log(LogLevelError, fmt.Sprintf("Required capability '%s' not found. Plugin exiting.", method))
		panic(fmt.Sprintf("missing required capability: %s", method))
	}
}

// KV Utils
func (p *Plugin) KVSet(key string, val []byte) bool {
	return p.host.KvSet(key, val)
}

func (p *Plugin) KVGet(key string) ([]byte, bool) {
	res := p.host.KvGet(key)
	if res.IsSome() {
		return res.Unwrap(), true
	}
	return nil, false
}

func (p *Plugin) KVDelete(key string) bool {
	return p.host.KvDelete(key)
}

func (p *Plugin) KVList(prefix string) []string {
	return p.host.KvList(prefix)
}

// Registry Utils
func (p *Plugin) RegisterCapability(cap AlloyCapability) {
	p.host.RegisterCapability(cap)
}

func (p *Plugin) UnregisterCapability(method string) {
	p.host.UnregisterCapability(method)
}

func (p *Plugin) FindProviders(method, actor string, contextID string) []string {
	return p.host.FindProviders(method, actor, contextID)
}

func (p *Plugin) GetAllCapabilities(actor string, contextID string) []AlloyCapability {
	return p.host.GetAllCapabilities(actor, contextID)
}

// Buffer Utils
func (p *Plugin) ReadBuffer(id string) (AlloyBuffer, bool) {
	res := p.host.ReadBuffer(id)
	if res.IsSome() {
		return res.Unwrap(), true
	}
	return AlloyBuffer{}, false
}

// ReadBufferShared returns the direct host memory for a buffer if available.
// This is the fastest possible data path.
func (p *Plugin) ReadBufferShared(id string) ([]byte, bool) {
	ptr, size, ok := p.host.GetBufferView(id)
	if !ok {
		return nil, false
	}

	// Implementation note: converting ptr/size to []byte safely requires
	// specific WASM memory flags. For now, we use unsafe to wrap the
	// allocated guest pointer.
	p.Log(LogLevelDebug, fmt.Sprintf("Shared buffer mapped in guest memory at %d with size %d", ptr, size))

	var data []byte
	header := (*struct {
		Data uintptr
		Len  int
		Cap  int
	})(unsafe.Pointer(&data))
	header.Data = uintptr(ptr)
	header.Len = int(size)
	header.Cap = int(size)

	return data, true
}

func (p *Plugin) WriteBuffer(id string, content []byte) bool {
	return p.host.WriteBuffer(id, content)
}

func (p *Plugin) ListBuffers() []string {
	return p.host.ListBuffers()
}

func (p *Plugin) GetBufferView(id string) (ptr, size uint32, ok bool) {
	return p.host.GetBufferView(id)
}

// Dashboard Utils
func (p *Plugin) RegisterWidget(w AlloyWidget) {
	p.host.RegisterWidget(w)
}

func (p *Plugin) UnregisterWidget(id string) {
	p.host.UnregisterWidget(id)
}

func (p *Plugin) UpdateWidget(id string, content []byte) {
	p.host.UpdateWidget(id, content)
}
