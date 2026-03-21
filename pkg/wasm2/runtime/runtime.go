package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
	"github.com/jnesbitt/alloy-go/pkg/wasm2/bindings/guest"
	"github.com/tetratelabs/wazero"
	wazeroapi "github.com/tetratelabs/wazero/api"
)

// Runtime manages the WASM runtime environment using WIT bindings.
type Runtime struct {
	runtime    wazero.Runtime
	logger     *slog.Logger
	kv         storage.StateStore
	dataDir    string
	routerFn   func(ctx context.Context, msg api.Message)
	callFn     func(ctx context.Context, msg api.Message) (api.Message, error)
	plugins    map[string]*Instance
	mu         sync.RWMutex
	hostModule wazeroapi.Module
}

// Instance represents a WASM plugin instance.
type Instance struct {
	id           string
	ctx          context.Context
	cancel       context.CancelFunc
	mod          wazeroapi.Module
	logger       *slog.Logger
	msgChan      chan api.Message
	respChan     chan api.Message
	capabilities []api.Capability
	status       Status
	witInstance  *guest.AlloyInstance
}

// Status represents the plugin's execution status.
type Status int

const (
	StatusRunning Status = iota
	StatusPaused
	StatusStopped
	StatusCrashed
)

// NewRuntime creates a new WIT-based WASM runtime.
func NewRuntime(
	ctx context.Context,
	logger *slog.Logger,
	kv storage.StateStore,
	dataDir string,
	router func(ctx context.Context, msg api.Message),
	call func(ctx context.Context, msg api.Message) (api.Message, error),
) (*Runtime, error) {
	r := wazero.NewRuntime(ctx)

	rt := &Runtime{
		runtime:  r,
		logger:   logger,
		kv:       kv,
		dataDir:  dataDir,
		routerFn: router,
		callFn:   call,
		plugins:  make(map[string]*Instance),
	}

	// Instantiate the host module with WIT bindings
	hostMod, err := rt.instantiateHostModule(ctx)
	if err != nil {
		return nil, err
	}
	rt.hostModule = hostMod

	return rt, nil
}

// instantiateHostModule creates the host module with WIT bindings.
func (r *Runtime) instantiateHostModule(ctx context.Context) (wazeroapi.Module, error) {
	// Create a builder for the alloy host module
	builder := r.runtime.NewHostModuleBuilder("alloy")

	// Register WIT host functions
	builder = r.registerWITFunctions(builder)

	// Instantiate the module
	return builder.Instantiate(ctx)
}

// registerWITFunctions registers the WIT interface functions with the host module.
func (r *Runtime) registerWITFunctions(builder wazero.HostModuleBuilder) wazero.HostModuleBuilder {
	// Register the WIT host functions
	return builder
		// Message handling
		.NewFunctionBuilder().WithFunc(r.witHandleMessage).Export("handle-message")
		.NewFunctionBuilder().WithFunc(r.witRouteMessage).Export("route-message")
		.NewFunctionBuilder().WithFunc(r.witCall).Export("call")
		.NewFunctionBuilder().WithFunc(r.witGetNextMessage).Export("get-next-message")
		.NewFunctionBuilder().WithFunc(r.witSendResponse).Export("send-response")
		
		// Logging
		.NewFunctionBuilder().WithFunc(r.witLog).Export("log")
		
		// KV Storage
		.NewFunctionBuilder().WithFunc(r.witKVSet).Export("kv-set")
		.NewFunctionBuilder().WithFunc(r.witKVGet).Export("kv-get")
		.NewFunctionBuilder().WithFunc(r.witKVDelete).Export("kv-delete")
		.NewFunctionBuilder().WithFunc(r.witKVList).Export("kv-list")
		
		// Lifecycle
		.NewFunctionBuilder().WithFunc(r.witInit).Export("init")
		.NewFunctionBuilder().WithFunc(r.witStarted).Export("started")
}

// LoadPlugin instantiates a WASM plugin with WIT bindings.
func (r *Runtime) LoadPlugin(
	ctx context.Context,
	id string,
	wasmBytes []byte,
	fuelLimit uint64,
	caps []api.Capability,
) (*Instance, error) {
	// Create plugin storage directory
	pluginDir := filepath.Join(r.dataDir, id)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return nil, err
	}

	// Configure the module
	config := wazero.NewModuleConfig()
	config = config.WithName(id)
	config = config.WithStdout(newLoggerWriter(r.logger, id, "stdout"))
	config = config.WithStderr(newLoggerWriter(r.logger, id, "stderr"))
	config = config.WithFS(os.DirFS(pluginDir))

	// Compile the module
	compiled, err := r.runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, err
	}

	// Create context for the instance
	instCtx, instCancel := context.WithCancel(ctx)

	// Instantiate the module
	mod, err := r.runtime.InstantiateModule(instCtx, compiled, config)
	if err != nil {
		instCancel()
		return nil, err
	}

	// Create the instance
	instance := &Instance{
		id:           id,
		ctx:          instCtx,
		cancel:       instCancel,
		mod:          mod,
		logger:       r.logger,
		msgChan:      make(chan api.Message, 32),
		respChan:     make(chan api.Message, 32),
		capabilities: caps,
		status:       StatusRunning,
	}

	// Register the instance
	r.mu.Lock()
	r.plugins[id] = instance
	r.mu.Unlock()

	// Create the WIT instance
	witInst, err := guest.NewAlloyInstance(mod)
	if err != nil {
		instance.Close(ctx)
		return nil, err
	}
	instance.witInstance = witInst

	// Initialize the plugin with its capabilities
	witCaps := convertToWITCapabilities(caps)
	witInst.AlloyInit(id, witCaps)

	return instance, nil
}

// Close shuts down the runtime.
func (r *Runtime) Close(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Close all plugin instances
	for _, instance := range r.plugins {
		instance.Close(ctx)
	}

	// Close the runtime
	return r.runtime.Close(ctx)
}

// Close shuts down the plugin instance.
func (i *Instance) Close(ctx context.Context) error {
	i.cancel()
	if i.mod != nil {
		return i.mod.Close(ctx)
	}
	return nil
}

// convertToWITCapabilities converts API capabilities to WIT capabilities.
func convertToWITCapabilities(caps []api.Capability) []guest.AlloyCapability {
	witCaps := make([]guest.AlloyCapability, len(caps))
	for i, cap := range caps {
		witCaps[i] = guest.AlloyCapability{
			Method:      cap.Method,
			Description: cap.Description,
			Shortcut:    guest.AlloyOption[string]{Value: cap.Shortcut, Set: cap.Shortcut != ""},
		}
		if len(cap.Annotations) > 0 {
			annotations := make([]guest.AlloyTuple2StringStringT, 0, len(cap.Annotations))
			for k, v := range cap.Annotations {
				annotations = append(annotations, guest.AlloyTuple2StringStringT{F0: k, F1: v})
			}
			witCaps[i].Annotations = guest.AlloyOption[[]guest.AlloyTuple2StringStringT]{
				Value: annotations,
				Set:   true,
			}
		}
	}
	return witCaps
}

// convertToAPIMessage converts WIT message to API message.
func convertToAPIMessage(witMsg guest.AlloyMessage) api.Message {
	var payload json.RawMessage
	if len(witMsg.Payload) > 0 {
		payload = json.RawMessage(witMsg.Payload)
	}

	apiMsg := api.Message{
		ID:        witMsg.Id,
		Type:      "request", // Default, will be overridden by caller
		Method:    witMsg.Method,
		Sender:    witMsg.Sender,
		Payload:   payload,
		Timestamp: time.Now().UnixNano(),
	}

	if witMsg.Target.Set {
		apiMsg.Target = witMsg.Target.Value
	}

	return apiMsg
}

// convertToWITMessage converts API message to WIT message.
func convertToWITMessage(apiMsg api.Message) guest.AlloyMessage {
	var payload []byte
	if len(apiMsg.Payload) > 0 {
		payload = []byte(apiMsg.Payload)
	}

	witMsg := guest.AlloyMessage{
		Id:       apiMsg.ID,
		Method:   apiMsg.Method,
		Sender:   apiMsg.Sender,
		Payload:  payload,
		Timestamp: uint64(apiMsg.Timestamp),
	}

	if apiMsg.Target != "" {
		witMsg.Target = guest.AlloyOption[string]{Value: apiMsg.Target, Set: true}
	}

	return witMsg
}

// WIT Host Function Implementations

// witInit initializes a plugin with its capabilities.
func (r *Runtime) witInit(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, capsPtr, capsLen uint32) {
	// Read the plugin ID from WASM memory
	idData, ok := mod.Memory().Read(idPtr, idLen)
	if !ok {
		r.logger.Error("failed to read plugin ID from memory")
		return
	}
	pluginID := string(idData)

	// Read the capabilities from WASM memory
	capsData, ok := mod.Memory().Read(capsPtr, capsLen)
	if !ok {
		r.logger.Error("failed to read capabilities from memory", "plugin", pluginID)
		return
	}

	// Unmarshal the capabilities
	var witCaps []guest.AlloyCapability
	if err := json.Unmarshal(capsData, &witCaps); err != nil {
		r.logger.Error("failed to unmarshal capabilities", "plugin", pluginID, "error", err)
		return
	}

	// Convert to API capabilities
	apiCaps := make([]api.Capability, len(witCaps))
	for i, witCap := range witCaps {
		apiCap := api.Capability{
			Method:      witCap.Method,
			Description: witCap.Description,
			Shortcut:    "",
		}

		if witCap.Shortcut.Set {
			apiCap.Shortcut = witCap.Shortcut.Value
		}

		if witCap.Annotations.Set {
			apiCap.Annotations = make(map[string]string)
			for _, annot := range witCap.Annotations.Value {
				apiCap.Annotations[annot.F0] = annot.F1
			}
		}

		apiCaps[i] = apiCap
	}

	r.mu.Lock()
	if instance, ok := r.plugins[pluginID]; ok {
		instance.capabilities = apiCaps
	}
	r.mu.Unlock()

	r.logger.Debug("plugin initialized", "id", pluginID, "capabilities", len(apiCaps))
}

// witHandleMessage handles an incoming message.
func (r *Runtime) witHandleMessage(ctx context.Context, mod wazeroapi.Module, msgPtr, msgLen uint32) uint32 {
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()

	if !ok {
		r.logger.Error("plugin instance not found", "plugin", mod.Name())
		return 0
	}

	// Read the message from WASM memory
	msgData, ok := mod.Memory().Read(msgPtr, msgLen)
	if !ok {
		r.logger.Error("failed to read message from memory", "plugin", mod.Name())
		return 0
	}

	// Unmarshal the WIT message
	var witMsg guest.AlloyMessage
	if err := json.Unmarshal(msgData, &witMsg); err != nil {
		r.logger.Error("failed to unmarshal message", "plugin", mod.Name(), "error", err)
		return 0
	}

	r.logger.Debug("handling message", "plugin", mod.Name(), "method", witMsg.Method, "id", witMsg.Id)

	// Convert to API message for internal processing
	apiMsg := convertToAPIMessage(witMsg)
	apiMsg.Type = "request"
	apiMsg.Sender = instance.id

	// For now, we'll just create a simple response
	// In a real implementation, this would call the plugin's handler
	respPayload := map[string]string{"result": "success", "method": witMsg.Method}
	respData, err := json.Marshal(respPayload)
	if err != nil {
		r.logger.Error("failed to marshal response", "plugin", mod.Name(), "error", err)
		return 0
	}

	// Create the response message
	resp := guest.AlloyMessage{
		Id:      witMsg.Id + "-response",
		Method:  witMsg.Method,
		Sender:  instance.id,
		Target:  guest.AlloyOption[string]{Value: witMsg.Sender, Set: true},
		Payload: respData,
	}

	// Marshal the response
	respBytes, err := json.Marshal(resp)
	if err != nil {
		r.logger.Error("failed to marshal response message", "plugin", mod.Name(), "error", err)
		return 0
	}

	// Allocate memory in the guest for the response
	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		r.logger.Error("cabi_realloc function not found", "plugin", mod.Name())
		return 0
	}

	res, err := alloc.Call(ctx, 0, 0, 1, uint64(len(respBytes)))
	if err != nil || len(res) == 0 {
		r.logger.Error("failed to allocate memory in guest", "plugin", mod.Name(), "error", err)
		return 0
	}

	// Write the response to guest memory
	if !mod.Memory().Write(uint32(res[0]), respBytes) {
		r.logger.Error("failed to write response to guest memory", "plugin", mod.Name())
		return 0
	}

	r.logger.Debug("message handled successfully", "plugin", mod.Name(), "method", witMsg.Method)

	return uint32(res[0])
}

// witRouteMessage routes a message to its target.
func (r *Runtime) witRouteMessage(ctx context.Context, mod wazeroapi.Module, msgPtr, msgLen uint32) {
	pluginID := mod.Name()

	r.mu.RLock()
	instance, ok := r.plugins[pluginID]
	r.mu.RUnlock()

	if !ok {
		r.logger.Error("plugin instance not found", "plugin", pluginID)
		return
	}

	// Read the message from WASM memory
	msgData, ok := mod.Memory().Read(msgPtr, msgLen)
	if !ok {
		r.logger.Error("failed to read message from memory", "plugin", pluginID)
		return
	}

	// Unmarshal the WIT message
	var witMsg guest.AlloyMessage
	if err := json.Unmarshal(msgData, &witMsg); err != nil {
		r.logger.Error("failed to unmarshal message", "plugin", pluginID, "error", err)
		return
	}

	r.logger.Debug("routing message", "plugin", pluginID, "method", witMsg.Method, "target", witMsg.Target)

	// Convert to API message
	apiMsg := convertToAPIMessage(witMsg)
	apiMsg.Type = "event"
	apiMsg.Sender = pluginID

	// Route the message
	r.routerFn(instance.ctx, apiMsg)
}

// witCall performs a synchronous call to another plugin.
func (r *Runtime) witCall(ctx context.Context, mod wazeroapi.Module, msgPtr, msgLen, respPtrPtr, respSizePtr uint32) uint32 {
	pluginID := mod.Name()

	r.mu.RLock()
	instance, ok := r.plugins[pluginID]
	r.mu.RUnlock()

	if !ok {
		r.logger.Error("plugin instance not found", "plugin", pluginID)
		return 1
	}

	// Read the message from WASM memory
	msgData, ok := mod.Memory().Read(msgPtr, msgLen)
	if !ok {
		r.logger.Error("failed to read message from memory", "plugin", pluginID)
		return 1
	}

	// Unmarshal the WIT message
	var witMsg guest.AlloyMessage
	if err := json.Unmarshal(msgData, &witMsg); err != nil {
		r.logger.Error("failed to unmarshal message", "plugin", pluginID, "error", err)
		return 1
	}

	r.logger.Debug("performing call", "plugin", pluginID, "method", witMsg.Method, "target", witMsg.Target)

	// Convert to API message
	apiMsg := convertToAPIMessage(witMsg)
	apiMsg.Type = "call"
	apiMsg.Sender = pluginID

	// Perform the call
	resp, err := r.callFn(instance.ctx, apiMsg)
	if err != nil {
		r.logger.Error("call failed", "plugin", pluginID, "error", err)
		return 1
	}

	// Convert response to WIT message
	witResp := convertToWITMessage(resp)

	// Marshal the response
	respData, err := json.Marshal(witResp)
	if err != nil {
		r.logger.Error("failed to marshal response", "plugin", pluginID, "error", err)
		return 1
	}

	// Allocate memory in the guest for the response
	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		r.logger.Error("cabi_realloc function not found", "plugin", pluginID)
		return 1
	}

	res, err := alloc.Call(ctx, 0, 0, 1, uint64(len(respData)))
	if err != nil || len(res) == 0 {
		r.logger.Error("failed to allocate memory in guest", "plugin", pluginID, "error", err)
		return 1
	}

	// Write the response to guest memory
	if !mod.Memory().Write(uint32(res[0]), respData) {
		r.logger.Error("failed to write response to guest memory", "plugin", pluginID)
		return 1
	}

	// Write the response pointer and size
	if !mod.Memory().WriteUint32Le(respPtrPtr, uint32(res[0])) {
		r.logger.Error("failed to write response pointer", "plugin", pluginID)
		return 1
	}
	if !mod.Memory().WriteUint32Le(respSizePtr, uint32(len(respData))) {
		r.logger.Error("failed to write response size", "plugin", pluginID)
		return 1
	}

	r.logger.Debug("call completed successfully", "plugin", pluginID, "method", witMsg.Method)

	return 0
}

// witGetNextMessage gets the next message for the plugin.
func (r *Runtime) witGetNextMessage(ctx context.Context, mod wazeroapi.Module, ptrPtr, sizePtr uint32) uint32 {
	pluginID := mod.Name()

	r.mu.RLock()
	instance, ok := r.plugins[pluginID]
	r.mu.RUnlock()

	if !ok {
		r.logger.Error("plugin instance not found", "plugin", pluginID)
		return 0
	}

	select {
	case msg := <-instance.msgChan:
		r.logger.Debug("sending message to plugin", "plugin", pluginID, "method", msg.Method)
		
		// Convert to WIT message
		witMsg := convertToWITMessage(msg)
		
		// Marshal the message
		msgData, err := json.Marshal(witMsg)
		if err != nil {
			r.logger.Error("failed to marshal message", "plugin", pluginID, "error", err)
			return 0
		}

		// Allocate memory in the guest
		alloc := mod.ExportedFunction("cabi_realloc")
		if alloc == nil {
			r.logger.Error("cabi_realloc function not found", "plugin", pluginID)
			return 0
		}
		
		res, err := alloc.Call(ctx, 0, 0, 1, uint64(len(msgData)))
		if err != nil || len(res) == 0 {
			r.logger.Error("failed to allocate memory in guest", "plugin", pluginID, "error", err)
			return 0
		}
		
		// Write the message to guest memory
		if !mod.Memory().Write(uint32(res[0]), msgData) {
			r.logger.Error("failed to write message to guest memory", "plugin", pluginID)
			return 0
		}
		
		// Write the pointer and size
		if !mod.Memory().WriteUint32Le(ptrPtr, uint32(res[0])) {
			r.logger.Error("failed to write message pointer", "plugin", pluginID)
			return 0
		}
		if !mod.Memory().WriteUint32Le(sizePtr, uint32(len(msgData))) {
			r.logger.Error("failed to write message size", "plugin", pluginID)
			return 0
		}
		
		return 1 // Success
	case <-ctx.Done():
		r.logger.Debug("context done while waiting for message", "plugin", pluginID)
		return 0
	case <-time.After(10 * time.Millisecond):
		// Timeout, no message available
		return 0
	}
}

// witSendResponse sends a response from the plugin.
func (r *Runtime) witSendResponse(ctx context.Context, mod wazeroapi.Module, msgPtr, msgLen uint32) {
	pluginID := mod.Name()

	r.mu.RLock()
	instance, ok := r.plugins[pluginID]
	r.mu.RUnlock()

	if !ok {
		r.logger.Error("plugin instance not found", "plugin", pluginID)
		return
	}

	// Handle empty response
	if msgLen == 0 {
		select {
		case instance.respChan <- api.Message{}:
			r.logger.Debug("sent empty response", "plugin", pluginID)
		default:
			r.logger.Warn("response channel full", "plugin", pluginID)
		}
		return
	}

	// Read the message from WASM memory
	msgData, ok := mod.Memory().Read(msgPtr, msgLen)
	if !ok {
		r.logger.Error("failed to read message from memory", "plugin", pluginID)
		return
	}

	// Unmarshal the WIT message
	var witMsg guest.AlloyMessage
	if err := json.Unmarshal(msgData, &witMsg); err != nil {
		r.logger.Error("failed to unmarshal message", "plugin", pluginID, "error", err)
		return
	}

	r.logger.Debug("received response from plugin", "plugin", pluginID, "id", witMsg.Id, "method", witMsg.Method)

	// Convert to API message
	apiMsg := convertToAPIMessage(witMsg)
	apiMsg.Type = "response"

	// Send the response
	select {
	case instance.respChan <- apiMsg:
		r.logger.Debug("response sent successfully", "plugin", pluginID, "id", witMsg.Id)
	case <-time.After(100 * time.Millisecond):
		r.logger.Warn("response channel full, dropping response", "plugin", pluginID, "id", witMsg.Id)
	}
}

// witLog logs a message from the plugin.
func (r *Runtime) witLog(ctx context.Context, mod wazeroapi.Module, levelPtr, levelLen, msgPtr, msgLen uint32) {
	pluginID := mod.Name()

	// Read the level from WASM memory
	levelData, ok := mod.Memory().Read(levelPtr, levelLen)
	if !ok {
		r.logger.Error("failed to read log level from memory", "plugin", pluginID)
		return
	}
	level := string(levelData)

	// Read the message from WASM memory
	msgData, ok := mod.Memory().Read(msgPtr, msgLen)
	if !ok {
		r.logger.Error("failed to read log message from memory", "plugin", pluginID)
		return
	}
	msg := string(msgData)

	// Log based on level
	switch level {
	case "debug":
		r.logger.Debug("wasm_log", "plugin", pluginID, "msg", msg)
	case "info":
		r.logger.Info("wasm_log", "plugin", pluginID, "msg", msg)
	case "warn":
		r.logger.Warn("wasm_log", "plugin", pluginID, "msg", msg)
	case "error":
		r.logger.Error("wasm_log", "plugin", pluginID, "msg", msg)
	default:
		r.logger.Info("wasm_log", "plugin", pluginID, "level", level, "msg", msg)
	}
}

// witKVSet sets a key-value pair in storage.
func (r *Runtime) witKVSet(ctx context.Context, mod wazeroapi.Module, keyPtr, keyLen, valuePtr, valueLen uint32) uint32 {
	pluginID := mod.Name()

	// Read the key from WASM memory
	keyData, ok := mod.Memory().Read(keyPtr, keyLen)
	if !ok {
		r.logger.Error("failed to read key from memory", "plugin", pluginID)
		return 1
	}
	key := string(keyData)

	// Read the value from WASM memory
	var valueData []byte
	if valueLen > 0 {
		valueData, ok = mod.Memory().Read(valuePtr, valueLen)
		if !ok {
			r.logger.Error("failed to read value from memory", "plugin", pluginID, "key", key)
			return 1
		}
	}

	r.logger.Debug("setting KV pair", "plugin", pluginID, "key", key, "value_size", len(valueData))

	// Set the value in storage
	if err := r.kv.Set(pluginID, key, valueData); err != nil {
		r.logger.Error("failed to set KV pair", "plugin", pluginID, "key", key, "error", err)
		return 1
	}

	return 0
}

// witKVGet gets a value from storage.
func (r *Runtime) witKVGet(ctx context.Context, mod wazeroapi.Module, keyPtr, keyLen, valuePtrPtr, valueSizePtr uint32) uint32 {
	pluginID := mod.Name()

	// Read the key from WASM memory
	keyData, ok := mod.Memory().Read(keyPtr, keyLen)
	if !ok {
		r.logger.Error("failed to read key from memory", "plugin", pluginID)
		return 1
	}
	key := string(keyData)

	r.logger.Debug("getting KV value", "plugin", pluginID, "key", key)

	// Get the value from storage
	value, err := r.kv.Get(pluginID, key)
	if err != nil {
		r.logger.Error("failed to get KV value", "plugin", pluginID, "key", key, "error", err)
		return 1
	}

	// If no value, return 0 (not found)
	if value == nil {
		return 0
	}

	// Allocate memory in the guest for the value
	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		r.logger.Error("cabi_realloc function not found", "plugin", pluginID)
		return 1
	}

	res, err := alloc.Call(ctx, 0, 0, 1, uint64(len(value)))
	if err != nil || len(res) == 0 {
		r.logger.Error("failed to allocate memory in guest", "plugin", pluginID, "error", err)
		return 1
	}

	// Write the value to guest memory
	if !mod.Memory().Write(uint32(res[0]), value) {
		r.logger.Error("failed to write value to guest memory", "plugin", pluginID)
		return 1
	}

	// Write the pointer and size
	if !mod.Memory().WriteUint32Le(valuePtrPtr, uint32(res[0])) {
		r.logger.Error("failed to write value pointer", "plugin", pluginID)
		return 1
	}
	if !mod.Memory().WriteUint32Le(valueSizePtr, uint32(len(value))) {
		r.logger.Error("failed to write value size", "plugin", pluginID)
		return 1
	}

	r.logger.Debug("KV value retrieved", "plugin", pluginID, "key", key, "value_size", len(value))

	return 0
}

// witKVDelete deletes a key from storage.
func (r *Runtime) witKVDelete(ctx context.Context, mod wazeroapi.Module, keyPtr, keyLen uint32) uint32 {
	pluginID := mod.Name()

	// Read the key from WASM memory
	keyData, ok := mod.Memory().Read(keyPtr, keyLen)
	if !ok {
		r.logger.Error("failed to read key from memory", "plugin", pluginID)
		return 1
	}
	key := string(keyData)

	r.logger.Debug("deleting KV key", "plugin", pluginID, "key", key)

	// Delete the key from storage
	if err := r.kv.Delete(pluginID, key); err != nil {
		r.logger.Error("failed to delete KV key", "plugin", pluginID, "key", key, "error", err)
		return 1
	}

	return 0
}

// witKVList lists keys with a prefix.
func (r *Runtime) witKVList(ctx context.Context, mod wazeroapi.Module, prefixPtr, prefixLen, keysPtrPtr, keysSizePtr uint32) uint32 {
	pluginID := mod.Name()

	// Read the prefix from WASM memory
	prefixData, ok := mod.Memory().Read(prefixPtr, prefixLen)
	if !ok {
		r.logger.Error("failed to read prefix from memory", "plugin", pluginID)
		return 1
	}
	prefix := string(prefixData)

	r.logger.Debug("listing KV keys with prefix", "plugin", pluginID, "prefix", prefix)

	// List keys with the prefix
	keys, err := r.kv.List(pluginID, prefix)
	if err != nil {
		r.logger.Error("failed to list KV keys", "plugin", pluginID, "prefix", prefix, "error", err)
		return 1
	}

	// Marshal the keys
	keysData, err := json.Marshal(keys)
	if err != nil {
		r.logger.Error("failed to marshal keys", "plugin", pluginID, "error", err)
		return 1
	}

	// Allocate memory in the guest for the keys
	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		r.logger.Error("cabi_realloc function not found", "plugin", pluginID)
		return 1
	}

	res, err := alloc.Call(ctx, 0, 0, 1, uint64(len(keysData)))
	if err != nil || len(res) == 0 {
		r.logger.Error("failed to allocate memory in guest", "plugin", pluginID, "error", err)
		return 1
	}

	// Write the keys to guest memory
	if !mod.Memory().Write(uint32(res[0]), keysData) {
		r.logger.Error("failed to write keys to guest memory", "plugin", pluginID)
		return 1
	}

	// Write the pointer and size
	if !mod.Memory().WriteUint32Le(keysPtrPtr, uint32(res[0])) {
		r.logger.Error("failed to write keys pointer", "plugin", pluginID)
		return 1
	}
	if !mod.Memory().WriteUint32Le(keysSizePtr, uint32(len(keysData))) {
		r.logger.Error("failed to write keys size", "plugin", pluginID)
		return 1
	}

	r.logger.Debug("KV keys listed", "plugin", pluginID, "prefix", prefix, "count", len(keys))

	return 0
}

// witStarted signals that the plugin is ready.
func (r *Runtime) witStarted(ctx context.Context, mod wazeroapi.Module) {
	pluginID := mod.Name()
	r.logger.Info("WASM plugin ready", "plugin", pluginID)

	r.mu.Lock()
	if instance, ok := r.plugins[pluginID]; ok {
		instance.status = StatusRunning
	}
	r.mu.Unlock()
}

// RouteMessage routes a message to a plugin.
func (r *Runtime) RouteMessage(ctx context.Context, pluginID string, msg api.Message) error {
	r.mu.RLock()
	instance, ok := r.plugins[pluginID]
	r.mu.RUnlock()

	if !ok {
		return errors.New("plugin not found")
	}

	select {
	case instance.msgChan <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return errors.New("message channel full")
	}
}

// GetResponse gets a response from a plugin.
func (r *Runtime) GetResponse(ctx context.Context, pluginID string) (api.Message, error) {
	r.mu.RLock()
	instance, ok := r.plugins[pluginID]
	r.mu.RUnlock()

	if !ok {
		return api.Message{}, errors.New("plugin not found")
	}

	select {
	case resp := <-instance.respChan:
		return resp, nil
	case <-ctx.Done():
		return api.Message{}, ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return api.Message{}, errors.New("no response available")
	}
}

// loggerWriter handles WASM plugin stdout/stderr.
type loggerWriter struct {
	logger *slog.Logger
	id     string
	stream string
	buf    []byte
}

func newLoggerWriter(logger *slog.Logger, id, stream string) *loggerWriter {
	return &loggerWriter{
		logger: logger,
		id:     id,
		stream: stream,
	}
}

func (l *loggerWriter) Write(p []byte) (n int, err error) {
	l.buf = append(l.buf, p...)
	for {
		idx := -1
		for i, b := range l.buf {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx == -1 {
			break
		}
		line := string(l.buf[:idx])
		l.buf = l.buf[idx+1:]
		if l.stream == "stdout" {
			l.logger.Info("wasm_stdout", "plugin", l.id, "msg", line)
		} else {
			l.logger.Info("wasm_stderr", "plugin", l.id, "msg", line)
		}
	}
	return len(p), nil
}