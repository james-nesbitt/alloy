package runtime

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
	"github.com/tetratelabs/wazero"
	wazeroapi "github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

var i32le = binary.LittleEndian

// Runtime manages the WASM runtime environment.
type Runtime struct {
	baseRuntime      wazero.Runtime
	rtConfig         wazero.RuntimeConfig
	compilationCache wazero.CompilationCache
	logger           *slog.Logger
	kv               storage.StateStore
	dataDir          string
	routerFn         func(ctx context.Context, msg api.Message)
	callFn           func(ctx context.Context, msg api.Message) (api.Message, error)
	plugins          map[string]*Instance
	mu               sync.RWMutex
	hostModule       wazeroapi.Module

	// Integrated Buffer Management
	buffers api.MmapRegistry

	// Workspace management
	workspaces      map[string]api.Workspace
	activeWorkspace string

	// Buffer Views (pluginID -> bufferID -> guestPtr)
	bufferViews map[string]map[string]uint32

	// Dashboard management
	widgets map[string]api.Widget
}

// Instance represents a WASM plugin instance.
type Instance struct {
	id            string
	ctx           context.Context
	cancel        context.CancelFunc
	mod           wazeroapi.Module
	pluginRuntime wazero.Runtime
	logger        *slog.Logger
	msgChan       chan api.Message
	capabilities  []api.Capability
	status        Status
	metadata      api.PluginMetadata

	startedCh chan struct{}

	// pending responses: msgID -> channel
	pmu     sync.Mutex
	pending map[string]chan api.Message

	// Resource limits
	maxMemoryBytes uint32
	msgPerSecond   int
	bytesPerSecond int // New byte rate limit
	fuelLimit      int // Execution limit proxy (ms)

	// Throttling state
	msgCount     int
	byteCount    int
	lastMsgReset time.Time
	rmu          sync.Mutex

	// Circuit breaker
	crashCount  int
	lastCrash   time.Time
	circuitOpen bool
}

func (i *Instance) checkThrottle(msgSize int) error {
	if i.msgPerSecond <= 0 && i.bytesPerSecond <= 0 {
		return nil
	}

	i.rmu.Lock()
	defer i.rmu.Unlock()

	if i.circuitOpen {
		return fmt.Errorf("plugin circuit breaker is OPEN (too many crashes)")
	}

	now := time.Now()
	if now.Sub(i.lastMsgReset) > time.Second {
		i.msgCount = 0
		i.byteCount = 0
		i.lastMsgReset = now
	}

	if i.msgPerSecond > 0 && i.msgCount >= i.msgPerSecond {
		return fmt.Errorf("message rate limit exceeded (max %d/sec)", i.msgPerSecond)
	}
	if i.bytesPerSecond > 0 && i.byteCount+msgSize >= i.bytesPerSecond {
		return fmt.Errorf("byte rate limit exceeded (max %d bytes/sec)", i.bytesPerSecond)
	}

	i.msgCount++
	i.byteCount += msgSize
	return nil
}

func (i *Instance) recordCrash() {
	i.rmu.Lock()
	defer i.rmu.Unlock()

	now := time.Now()
	if now.Sub(i.lastCrash) > 60*time.Second {
		i.crashCount = 0
	}

	i.crashCount++
	i.lastCrash = now
	if i.crashCount >= 3 {
		i.circuitOpen = true
		i.status = StatusCrashed
	}
}

// Metadata returns the plugin's metadata.
func (i *Instance) Metadata() api.PluginMetadata {
	return i.metadata
}

// Close closes the plugin instance.
func (i *Instance) Close(ctx context.Context) error {
	i.cancel()
	if i.mod != nil {
		_ = i.mod.Close(ctx)
	}
	if i.pluginRuntime != nil {
		return i.pluginRuntime.Close(ctx)
	}
	return nil
}

// Status represents the plugin's execution status.
type Status int

const (
	StatusRunning Status = iota
	StatusPaused
	StatusStopped
	StatusCrashed
)

// NewRuntime creates a new WASM runtime.
func NewRuntime(
	ctx context.Context,
	logger *slog.Logger,
	kv storage.StateStore,
	dataDir string,
	bufferRegistry api.MmapRegistry,
	router func(ctx context.Context, msg api.Message),
	call func(ctx context.Context, msg api.Message) (api.Message, error),
) (*Runtime, error) {
	cache := wazero.NewCompilationCache()
	rtConfig := wazero.NewRuntimeConfig().WithCompilationCache(cache)
	r := wazero.NewRuntimeWithConfig(ctx, rtConfig)
	logger.Info("creating new WIT-based runtime (v2.9-async-compile)")

	rt := &Runtime{
		baseRuntime:      r,
		rtConfig:         rtConfig,
		compilationCache: cache,
		logger:           logger,
		kv:               kv,
		dataDir:          dataDir,
		buffers:          bufferRegistry,
		routerFn:         router,
		callFn:           call,
		plugins:          make(map[string]*Instance),
		workspaces:       make(map[string]api.Workspace),
		bufferViews:      make(map[string]map[string]uint32),
		widgets:          make(map[string]api.Widget),
	}
	rt.loadWorkspaces()
	rt.loadWidgets()

	// Instantiate the host module into base with functions (for shared access if needed)
	hostMod, err := rt.instantiateHostModuleInRuntime(ctx, rt.baseRuntime)
	if err != nil {
		return nil, err
	}
	rt.hostModule = hostMod

	// Instantiate WASI into base
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt.baseRuntime); err != nil {
		return nil, fmt.Errorf("failed to instantiate WASI: %w", err)
	}

	// Instantiate asyncify (dummy)
	_, _ = rt.baseRuntime.NewHostModuleBuilder("asyncify").
		NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start").
		NewFunctionBuilder().WithFunc(func() {}).Export("stop").
		NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start_unwind").
		NewFunctionBuilder().WithFunc(func() {}).Export("stop_unwind").
		NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start_rewind").
		NewFunctionBuilder().WithFunc(func() {}).Export("stop_rewind").
		Instantiate(ctx)

	return rt, nil
}

// instantiateHostModuleInRuntime creates the host module with WIT functions in a specific runtime.
func (r *Runtime) instantiateHostModuleInRuntime(ctx context.Context, rt wazero.Runtime) (wazeroapi.Module, error) {
	builder := rt.NewHostModuleBuilder("alloy")

	builder.NewFunctionBuilder().WithFunc(r.internalInit).Export("init")
	builder.NewFunctionBuilder().WithFunc(r.internalHandleMessage).Export("handle-message")
	builder.NewFunctionBuilder().WithFunc(r.internalLog).Export("log")
	builder.NewFunctionBuilder().WithFunc(r.internalKVSet).Export("kv-set")
	builder.NewFunctionBuilder().WithFunc(r.internalKVGet).Export("kv-get")
	builder.NewFunctionBuilder().WithFunc(r.internalKVDelete).Export("kv-delete")
	builder.NewFunctionBuilder().WithFunc(r.internalKVList).Export("kv-list")
	builder.NewFunctionBuilder().WithFunc(r.internalRouteMessage).Export("route-message")
	builder.NewFunctionBuilder().WithFunc(r.internalCall).Export("call")
	builder.NewFunctionBuilder().WithFunc(r.internalStarted).Export("started")
	builder.NewFunctionBuilder().WithFunc(r.internalGetNextMessage).Export("get-next-message")
	builder.NewFunctionBuilder().WithFunc(r.internalSendResponse).Export("send-response")

	builder.NewFunctionBuilder().WithFunc(r.internalGetActiveWorkspace).Export("get-active-workspace")
	builder.NewFunctionBuilder().WithFunc(r.internalSetActiveWorkspace).Export("set-active-workspace")
	builder.NewFunctionBuilder().WithFunc(r.internalListWorkspaces).Export("list-workspaces")
	builder.NewFunctionBuilder().WithFunc(r.internalRegisterWorkspace).Export("register-workspace")
	builder.NewFunctionBuilder().WithFunc(r.internalUnregisterWorkspace).Export("unregister-workspace")

	// Registry & Direct Interaction (New)
	builder.NewFunctionBuilder().WithFunc(r.internalRegisterCapability).Export("register-capability")
	builder.NewFunctionBuilder().WithFunc(r.internalUnregisterCapability).Export("unregister-capability")
	builder.NewFunctionBuilder().WithFunc(r.internalFindProviders).Export("find-providers")
	builder.NewFunctionBuilder().WithFunc(r.internalGetAllCapabilities).Export("get-all-capabilities")

	builder.NewFunctionBuilder().WithFunc(r.internalReadBuffer).Export("read-buffer")
	builder.NewFunctionBuilder().WithFunc(r.internalWriteBuffer).Export("write-buffer")
	builder.NewFunctionBuilder().WithFunc(r.internalListBuffers).Export("list-buffers")
	builder.NewFunctionBuilder().WithFunc(r.internalGetBufferView).Export("get-buffer-view")

	// Dashboard/Widget Management
	builder.NewFunctionBuilder().WithFunc(r.internalRegisterWidget).Export("register-widget")
	builder.NewFunctionBuilder().WithFunc(r.internalUnregisterWidget).Export("unregister-widget")
	builder.NewFunctionBuilder().WithFunc(r.internalUpdateWidget).Export("update-widget")

	// Complex data types (save/load state) - currently placeholders
	builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod wazeroapi.Module, resPtr uint32) {
		mod.Memory().WriteUint32Le(resPtr, 0)
		mod.Memory().WriteUint32Le(resPtr+4, 0)
	}).Export("save-state")

	builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod wazeroapi.Module, ptr, len uint32) {}).Export("load-state")

	return builder.Instantiate(ctx)
}

// Internal host logic

func (r *Runtime) internalInit(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, capsPtr, capsLen uint32) {
	id := r.readStringFromArgs(mod, idPtr, idLen)

	if capsLen > 0 {
		// alloy_capability_t is 40 bytes
		data, _ := mod.Memory().Read(capsPtr, capsLen*40)
		for i := uint32(0); i < capsLen; i++ {
			ptr := uint32(i * 40)

			methodPtr := i32le.Uint32(data[ptr:])
			methodLen := i32le.Uint32(data[ptr+4:])
			descPtr := i32le.Uint32(data[ptr+8:])
			descLen := i32le.Uint32(data[ptr+12:])

			cap := api.Capability{
				Method:      r.readStringFromArgs(mod, methodPtr, methodLen),
				Description: r.readStringFromArgs(mod, descPtr, descLen),
			}

			shortSet := i32le.Uint32(data[ptr+16:])
			if shortSet != 0 {
				shortPtr := i32le.Uint32(data[ptr+20:])
				shortLen := i32le.Uint32(data[ptr+24:])
				cap.Shortcut = r.readStringFromArgs(mod, shortPtr, shortLen)
			}

			annoSet := i32le.Uint32(data[ptr+28:])
			if annoSet != 0 {
				annoPtr := i32le.Uint32(data[ptr+32:])
				annoLen := i32le.Uint32(data[ptr+36:])
				if annoLen > 0 {
					cap.Annotations = make(map[string]string)
					// metadata is a list of tuples, each tuple is 16 bytes
					metaData, _ := mod.Memory().Read(annoPtr, annoLen*16)
					for j := uint32(0); j < annoLen; j++ {
						kPtr := i32le.Uint32(metaData[j*16:])
						kLen := i32le.Uint32(metaData[j*16+4:])
						vPtr := i32le.Uint32(metaData[j*16+8:])
						vLen := i32le.Uint32(metaData[j*16+12:])

						k := r.readStringFromArgs(mod, kPtr, kLen)
						v := r.readStringFromArgs(mod, vPtr, vLen)
						cap.Annotations[k] = v
					}
				}
			}

			// Register in command-manager
			payload, _ := json.Marshal(cap)
			r.routerFn(ctx, api.Message{
				ID:      fmt.Sprintf("init-cap-%d-%d", time.Now().UnixNano(), i),
				Sender:  id,
				Target:  "command-manager",
				Method:  "register-capability",
				Payload: payload,
			})
		}
	}
}

func (r *Runtime) internalStarted(ctx context.Context, mod wazeroapi.Module) {
	r.logger.Info("wasm plugin ready", "plugin", mod.Name())
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()
	if ok && instance.startedCh != nil {
		select {
		case <-instance.startedCh:
		default:
			close(instance.startedCh)
		}
	}
}

func (r *Runtime) internalLog(ctx context.Context, mod wazeroapi.Module, levelPtr, levelLen, msgPtr, msgLen uint32) {
	levelData, _ := mod.Memory().Read(levelPtr, levelLen)
	msgData, _ := mod.Memory().Read(msgPtr, msgLen)
	r.logger.Info("plugin_log", "id", mod.Name(), "level", string(levelData), "msg", string(msgData))
}

func (r *Runtime) internalKVSet(ctx context.Context, mod wazeroapi.Module, keyPtr, keyLen, valuePtr, valueLen uint32) uint32 {
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()

	// Check if this plugin has a storage limit (simple heuristic based on maxMemory)
	if ok && instance.maxMemoryBytes > 0 {
		// Just a simple safety check: don't allow items larger than 1/4 of total memory
		if valueLen > instance.maxMemoryBytes/4 {
			r.logger.Error("kv-set size limit exceeded", "id", mod.Name(), "size", valueLen)
			return 0
		}
	}

	keyData, _ := mod.Memory().Read(keyPtr, keyLen)
	valueData, _ := mod.Memory().Read(valuePtr, valueLen)
	if err := r.kv.Set(mod.Name(), string(keyData), valueData); err != nil {
		return 0
	}
	return 1
}

func (r *Runtime) internalKVGet(ctx context.Context, mod wazeroapi.Module, keyPtr, keyLen, resultPtr uint32) {
	keyData, _ := mod.Memory().Read(keyPtr, keyLen)
	value, err := r.kv.Get(mod.Name(), string(keyData))
	if err != nil || value == nil {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		return
	}

	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		return
	}
	res, err := alloc.Call(ctx, 0, 0, 1, uint64(len(value)))
	if err != nil || len(res) == 0 {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		return
	}

	mod.Memory().Write(uint32(res[0]), value)
	mod.Memory().WriteUint32Le(resultPtr, 1)                    // is_some = true
	mod.Memory().WriteUint32Le(resultPtr+4, uint32(res[0]))     // ptr
	mod.Memory().WriteUint32Le(resultPtr+8, uint32(len(value))) // len
}

func (r *Runtime) internalKVDelete(ctx context.Context, mod wazeroapi.Module, keyPtr, keyLen uint32) uint32 {
	keyData, _ := mod.Memory().Read(keyPtr, keyLen)
	if err := r.kv.Delete(mod.Name(), string(keyData)); err != nil {
		return 0
	}
	return 1
}

func (r *Runtime) internalKVList(ctx context.Context, mod wazeroapi.Module, prefixPtr, prefixLen, resultPtr uint32) {
	prefixData, _ := mod.Memory().Read(prefixPtr, prefixLen)

	prefix := string(prefixData)
	keys, err := r.kv.List(mod.Name(), prefix)
	if err != nil {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		mod.Memory().WriteUint32Le(resultPtr+4, 0)
		return
	}

	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		return
	}

	stringStructs := make([]byte, len(keys)*8)
	for i, key := range keys {
		sRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(key)))
		if len(sRes) > 0 {
			mod.Memory().Write(uint32(sRes[0]), []byte(key))
			i32le.PutUint32(stringStructs[i*8:], uint32(sRes[0]))
			i32le.PutUint32(stringStructs[i*8+4:], uint32(len(key)))
		}
	}

	lRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(stringStructs)))
	if len(lRes) > 0 {
		mod.Memory().Write(uint32(lRes[0]), stringStructs)
		mod.Memory().WriteUint32Le(resultPtr, uint32(lRes[0]))
		mod.Memory().WriteUint32Le(resultPtr+4, uint32(len(keys)))
	}
}

// Message reading/writing helpers

func (r *Runtime) readStringFromArgs(mod wazeroapi.Module, ptr, length uint32) string {
	if length == 0 {
		return ""
	}
	data, _ := mod.Memory().Read(ptr, length)
	return string(data)
}

func (r *Runtime) readMessageFromArgs(
	mod wazeroapi.Module,
	idPtr, idLen, typePtr, typeLen, methodPtr, methodLen, senderPtr, senderLen uint32,
	targetSet, targetPtr, targetLen, payloadPtr, payloadLen uint32,
	timestamp int64,
) api.Message {
	msg := api.Message{
		ID:        r.readStringFromArgs(mod, idPtr, idLen),
		Type:      api.MessageType(r.readStringFromArgs(mod, typePtr, typeLen)),
		Method:    r.readStringFromArgs(mod, methodPtr, methodLen),
		Sender:    r.readStringFromArgs(mod, senderPtr, senderLen),
		Timestamp: timestamp,
	}
	// For AlloyMessage, the WIT-generated calling convention passes options as discrete args.
	// We need to check targetSet != 0.
	if targetSet != 0 {
		msg.Target = r.readStringFromArgs(mod, targetPtr, targetLen)
	}
	if payloadLen > 0 {
		data, _ := mod.Memory().Read(payloadPtr, payloadLen)
		msg.Payload = json.RawMessage(data)
	}
	return msg
}

func (r *Runtime) writeMessage(ctx context.Context, mod wazeroapi.Module, ptr uint32, msg api.Message) {
	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		return
	}

	writeStr := func(fieldPtr uint32, s string) {
		if s == "" {
			mod.Memory().WriteUint32Le(fieldPtr, 0)
			mod.Memory().WriteUint32Le(fieldPtr+4, 0)
			return
		}
		res, err := alloc.Call(ctx, 0, 0, 1, uint64(len(s)))
		if err != nil || len(res) == 0 {
			r.logger.Error("cabi_realloc failed in writeMessage", "id", mod.Name(), "error", err)
			return
		}
		mod.Memory().Write(uint32(res[0]), []byte(s))
		mod.Memory().WriteUint32Le(fieldPtr, uint32(res[0]))
		mod.Memory().WriteUint32Le(fieldPtr+4, uint32(len(s)))
	}

	writeStr(ptr, msg.ID)
	writeStr(ptr+8, string(msg.Type))
	writeStr(ptr+16, msg.Method)
	writeStr(ptr+24, msg.Sender)

	// Offset 32: target (alloy_option_string_t)
	// bool is_some (4 bytes)
	// alloy_string_t val (8 bytes, starts at 36)
	if msg.Target != "" {
		mod.Memory().WriteUint32Le(ptr+32, 1)
		writeStr(ptr+36, msg.Target)
	} else {
		mod.Memory().WriteUint32Le(ptr+32, 0)
		mod.Memory().WriteUint32Le(ptr+36, 0)
		mod.Memory().WriteUint32Le(ptr+40, 0)
	}

	// Offset 44: payload (alloy_list_u8_t - 8 bytes)
	if len(msg.Payload) > 0 {
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(msg.Payload)))
		mod.Memory().Write(uint32(res[0]), msg.Payload)
		mod.Memory().WriteUint32Le(ptr+44, uint32(res[0]))
		mod.Memory().WriteUint32Le(ptr+48, uint32(len(msg.Payload)))
	} else {
		mod.Memory().WriteUint32Le(ptr+44, 0)
		mod.Memory().WriteUint32Le(ptr+48, 0)
	}

	// Offset 56: timestamp (uint64_t - 8 bytes)
	mod.Memory().WriteUint64Le(ptr+56, uint64(msg.Timestamp))
}

func (r *Runtime) internalHandleMessage(
	ctx context.Context, mod wazeroapi.Module,
	idPtr, idLen, typePtr, typeLen, methodPtr, methodLen, senderPtr, senderLen uint32,
	targetSet, targetPtr, targetLen, payloadPtr, payloadLen uint32,
	timestamp int64, resultPtr uint32,
) {
	msg := r.readMessageFromArgs(mod, idPtr, idLen, typePtr, typeLen, methodPtr, methodLen, senderPtr, senderLen, targetSet, targetPtr, targetLen, payloadPtr, payloadLen, timestamp)
	r.writeMessage(ctx, mod, resultPtr, api.Message{ID: msg.ID + "-resp", Type: api.TypeResponse, Method: "unimplemented"})
}

func (r *Runtime) internalRouteMessage(
	ctx context.Context, mod wazeroapi.Module,
	idPtr, idLen, typePtr, typeLen, methodPtr, methodLen, senderPtr, senderLen uint32,
	targetSet, targetPtr, targetLen, payloadPtr, payloadLen uint32,
	timestamp int64,
) {
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()

	if ok {
		if err := instance.checkThrottle(int(payloadLen)); err != nil {
			r.logger.Warn("plugin message throttled", "id", mod.Name(), "error", err)
			return
		}
	}

	msg := r.readMessageFromArgs(mod, idPtr, idLen, typePtr, typeLen, methodPtr, methodLen, senderPtr, senderLen, targetSet, targetPtr, targetLen, payloadPtr, payloadLen, timestamp)
	r.routerFn(ctx, msg)
}

func (r *Runtime) internalCall(
	ctx context.Context, mod wazeroapi.Module,
	idPtr, idLen, typePtr, typeLen, methodPtr, methodLen, senderPtr, senderLen uint32,
	targetSet, targetPtr, targetLen, payloadPtr, payloadLen uint32,
	timestamp int64, resultPtr uint32,
) {
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()

	if ok {
		if err := instance.checkThrottle(int(payloadLen)); err != nil {
			r.logger.Warn("plugin call throttled", "id", mod.Name(), "error", err)
			r.writeMessage(ctx, mod, resultPtr, api.Message{
				ID:      "throttle-err",
				Type:    api.TypeResponse,
				Sender:  "kernel",
				Payload: []byte(fmt.Sprintf(`{"error":%q}`, err.Error())),
			})
			return
		}
	}

	apiMsg := r.readMessageFromArgs(mod, idPtr, idLen, typePtr, typeLen, methodPtr, methodLen, senderPtr, senderLen, targetSet, targetPtr, targetLen, payloadPtr, payloadLen, timestamp)

	// Implementation of "Fuel" proxy: limited execution time for kernel calls
	callCtx := ctx
	if instance != nil && instance.fuelLimit > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(instance.fuelLimit)*time.Millisecond)
		defer cancel()
	}

	resp, err := r.callFn(callCtx, apiMsg)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && instance != nil {
			instance.recordCrash()
		}
		resp = api.Message{
			ID:      apiMsg.ID + "-resp",
			Type:    api.TypeResponse,
			Method:  apiMsg.Method,
			Sender:  "kernel",
			Payload: []byte(fmt.Sprintf(`{"error":%q}`, err.Error())),
		}
	}
	r.writeMessage(ctx, mod, resultPtr, resp)
}

func (r *Runtime) internalGetNextMessage(ctx context.Context, mod wazeroapi.Module, resultPtr uint32) {
	pluginID := mod.Name()
	r.mu.RLock()
	instance, ok := r.plugins[pluginID]
	r.mu.RUnlock()

	if !ok {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		return
	}

	select {
	case msg := <-instance.msgChan:
		r.logger.Debug("wasm plugin pulled message", "id", pluginID, "msgID", msg.ID)

		mod.Memory().WriteUint32Le(resultPtr, 1) // is_some = true
		r.writeMessage(ctx, mod, resultPtr+8, msg)
	case <-time.After(100 * time.Millisecond):
		// No message after wait, return None
		mod.Memory().WriteUint32Le(resultPtr, 0) // is_some = false
	case <-ctx.Done():
		mod.Memory().WriteUint32Le(resultPtr, 0)
	}
}

func (r *Runtime) internalSendResponse(
	ctx context.Context, mod wazeroapi.Module,
	idPtr, idLen, typePtr, typeLen, methodPtr, methodLen, senderPtr, senderLen uint32,
	targetSet, targetPtr, targetLen, payloadPtr, payloadLen uint32,
	timestamp int64,
) {
	apiMsg := r.readMessageFromArgs(mod, idPtr, idLen, typePtr, typeLen, methodPtr, methodLen, senderPtr, senderLen, targetSet, targetPtr, targetLen, payloadPtr, payloadLen, timestamp)
	apiMsg.Type = api.TypeResponse

	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()

	if ok {
		instance.pmu.Lock()
		defer instance.pmu.Unlock()

		// Try to find a waiter for this response
		if ch, ok := instance.pending[apiMsg.ID]; ok {
			select {
			case ch <- apiMsg:
			default:
			}
			return
		}

		// check for suffix match (remove -resp)
		reqID := strings.TrimSuffix(apiMsg.ID, "-resp")
		if ch, ok := instance.pending[reqID]; ok {
			select {
			case ch <- apiMsg:
			default:
			}
			return
		}

		// Fallback: route through normal kernel loop
		go r.routerFn(ctx, apiMsg)
	}
}

func (r *Runtime) LoadPlugin(
	ctx context.Context,
	id string,
	wasmBytes []byte,
	maxMemoryMB uint32,
	msgPerSec int,
	caps []api.Capability,
) (*Instance, error) {
	pluginDir := filepath.Join(r.dataDir, id)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create plugin storage dir: %w", err)
	}

	instCtx, instCancel := context.WithCancel(context.Background())
	startedCh := make(chan struct{})

	instance := &Instance{
		id:           id,
		ctx:          instCtx,
		cancel:       instCancel,
		logger:       r.logger,
		msgChan:      make(chan api.Message, 1024),
		capabilities: caps,
		status:       StatusRunning,
		metadata: api.PluginMetadata{
			ID:           id,
			Capabilities: caps,
		},
		startedCh: startedCh,
		pending:   make(map[string]chan api.Message),

		maxMemoryBytes: maxMemoryMB * 1024 * 1024,
		msgPerSecond:   msgPerSec,
		bytesPerSecond: 10 * 1024 * 1024, // Default 10MB/s
		fuelLimit:      1000,             // Default 1s execution proxy (ms)
		lastMsgReset:   time.Now(),
	}

	r.mu.Lock()
	r.plugins[id] = instance
	r.mu.Unlock()

	r.logger.Debug("created new plugin instance", "id", id, "ptr", fmt.Sprintf("%p", instance))

	// Compilation and Instantion happen in background to avoid blocking other plugins or host boot
	go func() {
		r.logger.Debug("instantiating wasm module", "id", id)

		// Hardening: Per-plugin runtime for memory isolation
		maxPages := instance.maxMemoryBytes / 65536
		if maxPages == 0 && instance.maxMemoryBytes > 0 {
			maxPages = 1
		}
		if maxPages == 0 {
			maxPages = 2048 // Default 128MB limit
		}

		instRtConfig := wazero.NewRuntimeConfig().
			WithCompilationCache(r.compilationCache). // Reuse the shared cache
			WithMemoryLimitPages(maxPages)

		pluginRuntime := wazero.NewRuntimeWithConfig(instCtx, instRtConfig)
		instance.pluginRuntime = pluginRuntime

		r.logger.Debug("compiling wasm module", "id", id, "bytes", len(wasmBytes))
		compiled, err := pluginRuntime.CompileModule(instCtx, wasmBytes)
		if err != nil {
			r.logger.Error("failed to compile module", "id", id, "error", err)
			instCancel()
			return
		}

		// Register host functions into THIS plugin's runtime
		if _, err := r.instantiateHostModuleInRuntime(instCtx, pluginRuntime); err != nil {
			r.logger.Error("failed to instantiate host module in plugin runtime", "id", id, "error", err)
			instCancel()
			return
		}

		// Instantiate WASI into plugin runtime
		if _, err := wasi_snapshot_preview1.Instantiate(instCtx, pluginRuntime); err != nil {
			r.logger.Error("failed to instantiate WASI in plugin runtime", "id", id, "error", err)
			instCancel()
			return
		}

		// Instantiate asyncify (dummy)
		_, _ = pluginRuntime.NewHostModuleBuilder("asyncify").
			NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start").
			NewFunctionBuilder().WithFunc(func() {}).Export("stop").
			NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start_unwind").
			NewFunctionBuilder().WithFunc(func() {}).Export("stop_unwind").
			NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start_rewind").
			NewFunctionBuilder().WithFunc(func() {}).Export("stop_rewind").
			Instantiate(instCtx)

		// For the project manager plugin, we allow access to the base data directory
		// to enable automatic discovery of project workspaces.
		var fs wazero.FSConfig
		if id == "project" {
			fs = wazero.NewFSConfig().WithDirMount(r.dataDir, "/")
		} else {
			fs = wazero.NewFSConfig().WithDirMount(pluginDir, "/")
		}

		config := wazero.NewModuleConfig().
			WithName(id).
			WithStdout(newLoggerWriter(r.logger, id, "stdout")).
			WithStderr(newLoggerWriter(r.logger, id, "stderr")).
			WithFSConfig(fs)

		mod, err := pluginRuntime.InstantiateModule(instCtx, compiled, config)
		if err != nil {
			// Don't log error for normal context cancellation (shutdown)
			if !errors.Is(err, context.Canceled) {
				r.logger.Error("wasm module terminated with error", "id", id, "error", err)
				instance.recordCrash()
			}
			instCancel()
			return
		}

		r.mu.Lock()
		instance.mod = mod
		r.mu.Unlock()
	}()

	// Wait for the plugin to signal it's ready (via internalStarted)
	// We wait up to 10 seconds for initial progress, but don't block boot loop forever
	select {
	case <-startedCh:
		r.logger.Info("plugin initialization signal received", "id", id)
	case <-time.After(10 * time.Second):
		r.logger.Warn("plugin initialization timed out, continuing anyway", "id", id)
	}

	// Initial Load: check if we have metadata in store
	go func() {
		time.Sleep(200 * time.Millisecond)
		metadataJSON, err := r.kv.Get(id, "plugin:metadata:"+id)
		if err == nil && metadataJSON != nil {
			var meta api.PluginMetadata
			if err := json.Unmarshal(metadataJSON, &meta); err == nil {
				r.mu.Lock()
				instance.metadata = meta
				r.mu.Unlock()
			}
		}
	}()

	return instance, nil
}

func (r *Runtime) Close(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, instance := range r.plugins {
		_ = instance.Close(ctx)
	}

	return r.baseRuntime.Close(ctx)
}

func (r *Runtime) UnloadPlugin(ctx context.Context, id string) error {
	r.mu.Lock()
	instance, ok := r.plugins[id]
	if ok {
		delete(r.plugins, id)
	}
	r.mu.Unlock()

	if ok {
		instance.cancel()
		if instance.mod != nil {
			return instance.mod.Close(ctx)
		}
		return nil
	}
	return nil
}

func (r *Runtime) RouteMessage(ctx context.Context, pluginID string, msg api.Message) error {
	r.mu.RLock()
	instance, ok := r.plugins[pluginID]
	r.mu.RUnlock()
	if !ok {
		return errors.New("plugin not found")
	}

	// Pre-register response channel if it's a request
	if msg.Type == api.TypeRequest {
		respCh := make(chan api.Message, 1)
		instance.pmu.Lock()
		instance.pending[msg.ID] = respCh
		instance.pmu.Unlock()
	}

	select {
	case instance.msgChan <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errors.New("channel full")
	}
}

func (r *Runtime) GetResponse(ctx context.Context, pluginID string, requestID string) (api.Message, error) {
	r.mu.RLock()
	instance, ok := r.plugins[pluginID]
	r.mu.RUnlock()
	if !ok {
		return api.Message{}, errors.New("plugin not found")
	}

	instance.pmu.Lock()
	respCh, ok := instance.pending[requestID]
	instance.pmu.Unlock()

	if !ok {
		// FALLBACK: If it wasn't pre-registered (unlikely now), create it.
		respCh = make(chan api.Message, 1)
		instance.pmu.Lock()
		instance.pending[requestID] = respCh
		instance.pmu.Unlock()
	}

	defer func() {
		instance.pmu.Lock()
		delete(instance.pending, requestID)
		instance.pmu.Unlock()
	}()

	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		return api.Message{}, ctx.Err()
	case <-time.After(15 * time.Second):
		return api.Message{}, errors.New("timeout waiting for WASM response")
	}
}

type loggerWriter struct {
	logger *slog.Logger
	id     string
	stream string
	buf    []byte
}

func newLoggerWriter(logger *slog.Logger, id, stream string) *loggerWriter {
	return &loggerWriter{logger: logger, id: id, stream: stream}
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
			l.logger.Info("wasm_out", "id", l.id, "msg", line)
		}
		if l.stream == "stderr" {
			l.logger.Error("wasm_err", "id", l.id, "msg", line)
		}
	}
	return len(p), nil
}

// Workspace WIT implementation

func (r *Runtime) writeWorkspace(ctx context.Context, mod wazeroapi.Module, ptr uint32, ws api.Workspace) {
	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		return
	}

	writeStr := func(fieldPtr uint32, s string) {
		if s == "" {
			mod.Memory().WriteUint32Le(fieldPtr, 0)
			mod.Memory().WriteUint32Le(fieldPtr+4, 0)
			return
		}
		res, err := alloc.Call(ctx, 0, 0, 1, uint64(len(s)))
		if err != nil || len(res) == 0 {
			return
		}
		mod.Memory().Write(uint32(res[0]), []byte(s))
		mod.Memory().WriteUint32Le(fieldPtr, uint32(res[0]))
		mod.Memory().WriteUint32Le(fieldPtr+4, uint32(len(s)))
	}

	writeStr(ptr, ws.ID)      // offset 0
	writeStr(ptr+8, ws.Name)  // offset 8
	writeStr(ptr+16, ws.Path) // offset 16

	// offset 24: team_id (option<string>)
	// option<string> is bool (4) + alloy_string_t (8) = 12 bytes
	if ws.TeamID != "" {
		mod.Memory().WriteUint32Le(ptr+24, 1) // some
		writeStr(ptr+28, ws.TeamID)
	} else {
		mod.Memory().WriteUint32Le(ptr+24, 0) // none
		mod.Memory().WriteUint32Le(ptr+28, 0)
		mod.Memory().WriteUint32Le(ptr+32, 0)
	}

	// offset 36: layout (option<string>)
	if ws.Layout != "" {
		mod.Memory().WriteUint32Le(ptr+36, 1) // some
		writeStr(ptr+40, ws.Layout)
	} else {
		mod.Memory().WriteUint32Le(ptr+36, 0)
		mod.Memory().WriteUint32Le(ptr+40, 0)
		mod.Memory().WriteUint32Le(ptr+44, 0)
	}

	// offset 48: metadata (list<tuple<string, string>>)
	// list is ptr (4) + len (4) = 8 bytes
	if len(ws.Metadata) > 0 {
		metaData := make([]byte, len(ws.Metadata)*16)
		i := 0
		for k, v := range ws.Metadata {
			vStr := fmt.Sprintf("%v", v)
			// Write key
			keyRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(k)))
			mod.Memory().Write(uint32(keyRes[0]), []byte(k))
			i32le.PutUint32(metaData[i*16:], uint32(keyRes[0]))
			i32le.PutUint32(metaData[i*16+4:], uint32(len(k)))

			// Write value
			valRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(vStr)))
			mod.Memory().Write(uint32(valRes[0]), []byte(vStr))
			i32le.PutUint32(metaData[i*16+8:], uint32(valRes[0]))
			i32le.PutUint32(metaData[i*16+12:], uint32(len(vStr)))
			i++
		}
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(metaData)))
		mod.Memory().Write(uint32(res[0]), metaData)
		mod.Memory().WriteUint32Le(ptr+48, uint32(res[0]))
		mod.Memory().WriteUint32Le(ptr+52, uint32(len(ws.Metadata)))
	} else {
		mod.Memory().WriteUint32Le(ptr+48, 0)
		mod.Memory().WriteUint32Le(ptr+52, 0)
	}
}

func (r *Runtime) internalGetActiveWorkspace(ctx context.Context, mod wazeroapi.Module, resultPtr uint32) {
	r.mu.RLock()
	ws, ok := r.workspaces[r.activeWorkspace]
	r.mu.RUnlock()

	if !ok {
		mod.Memory().WriteUint32Le(resultPtr, 0) // is_some = false
		return
	}

	mod.Memory().WriteUint32Le(resultPtr, 1)    // is_some = true
	r.writeWorkspace(ctx, mod, resultPtr+4, ws) // Offset 4 because alloy_option_workspace_t has bool is_some at start
}

func (r *Runtime) internalSetActiveWorkspace(ctx context.Context, mod wazeroapi.Module, idPtr, idLen uint32) {
	id := r.readStringFromArgs(mod, idPtr, idLen)
	r.mu.Lock()
	r.activeWorkspace = id
	r.mu.Unlock()
	r.saveWorkspaces()
}

func (r *Runtime) internalListWorkspaces(ctx context.Context, mod wazeroapi.Module, resultPtr uint32) {
	r.mu.RLock()
	workspaces := make([]api.Workspace, 0, len(r.workspaces))
	for _, ws := range r.workspaces {
		workspaces = append(workspaces, ws)
	}
	r.mu.RUnlock()

	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		return
	}

	// alloy_workspace_t is 56 bytes
	res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(workspaces)*56))
	basePtr := uint32(res[0])
	for i, ws := range workspaces {
		r.writeWorkspace(ctx, mod, basePtr+uint32(i*56), ws)
	}

	mod.Memory().WriteUint32Le(resultPtr, basePtr)
	mod.Memory().WriteUint32Le(resultPtr+4, uint32(len(workspaces)))
}

func (r *Runtime) internalRegisterWorkspace(
	ctx context.Context, mod wazeroapi.Module,
	idPtr, idLen, namePtr, nameLen, pathPtr, pathLen uint32,
	teamIDSet, teamIDPtr, teamIDLen uint32,
	layoutSet, layoutPtr, layoutLen uint32,
	metadataPtr, metadataLen uint32,
) {
	ws := api.Workspace{
		ID:   r.readStringFromArgs(mod, idPtr, idLen),
		Name: r.readStringFromArgs(mod, namePtr, nameLen),
		Path: r.readStringFromArgs(mod, pathPtr, pathLen),
	}

	if teamIDSet != 0 {
		ws.TeamID = r.readStringFromArgs(mod, teamIDPtr, teamIDLen)
	}

	if layoutSet != 0 {
		ws.Layout = r.readStringFromArgs(mod, layoutPtr, layoutLen)
	}

	if metadataLen > 0 {
		ws.Metadata = make(map[string]string)
		// metadata is a list of tuples, each tuple is 16 bytes
		metaData, _ := mod.Memory().Read(metadataPtr, metadataLen*16)
		for i := uint32(0); i < metadataLen; i++ {
			kPtr := i32le.Uint32(metaData[i*16:])
			kLen := i32le.Uint32(metaData[i*16+4:])
			vPtr := i32le.Uint32(metaData[i*16+8:])
			vLen := i32le.Uint32(metaData[i*16+12:])

			k := r.readStringFromArgs(mod, kPtr, kLen)
			v := r.readStringFromArgs(mod, vPtr, vLen)
			ws.Metadata[k] = v
		}
	}

	r.mu.Lock()
	r.workspaces[ws.ID] = ws
	// If no active workspace, make this one active
	if r.activeWorkspace == "" {
		r.activeWorkspace = ws.ID
	}
	r.mu.Unlock()
	r.saveWorkspaces()
}

func (r *Runtime) internalUnregisterWorkspace(ctx context.Context, mod wazeroapi.Module, idPtr, idLen uint32) {
	id := r.readStringFromArgs(mod, idPtr, idLen)
	r.mu.Lock()
	delete(r.workspaces, id)
	if r.activeWorkspace == id {
		r.activeWorkspace = ""
		// Pick another one if available
		for nextID := range r.workspaces {
			r.activeWorkspace = nextID
			break
		}
	}
	r.mu.Unlock()
	r.saveWorkspaces()
}

// Persistence for workspaces
func (r *Runtime) saveWorkspaces() {
	r.mu.RLock()
	data, _ := json.Marshal(r.workspaces)
	active := r.activeWorkspace
	r.mu.RUnlock()

	_ = r.kv.Set("system", "workspaces", data)
	_ = r.kv.Set("system", "active_workspace", []byte(active))
}

func (r *Runtime) loadWorkspaces() {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.kv.Get("system", "workspaces")
	if err == nil && data != nil {
		_ = json.Unmarshal(data, &r.workspaces)
	}

	active, err := r.kv.Get("system", "active_workspace")
	if err == nil && active != nil {
		r.activeWorkspace = string(active)
	}
}

// Persistence for widgets
func (r *Runtime) saveWidgets() {
	r.mu.RLock()
	data, _ := json.Marshal(r.widgets)
	r.mu.RUnlock()

	_ = r.kv.Set("system", "widgets", data)
}

func (r *Runtime) loadWidgets() {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.kv.Get("system", "widgets")
	if err == nil && data != nil {
		_ = json.Unmarshal(data, &r.widgets)
	}
}

// Registry & Direct Interaction implemention

func (r *Runtime) internalRegisterCapability(ctx context.Context, mod wazeroapi.Module, methodPtr, methodLen, descPtr, descLen, shortcutSet, shortcutPtr, shortcutLen, annoSet, annoPtr, annoLen uint32) {
	method := r.readStringFromArgs(mod, methodPtr, methodLen)
	desc := r.readStringFromArgs(mod, descPtr, descLen)
	cap := api.Capability{
		Method:      method,
		Description: desc,
	}
	if shortcutSet != 0 {
		cap.Shortcut = r.readStringFromArgs(mod, shortcutPtr, shortcutLen)
	}

	if annoSet != 0 && annoLen > 0 {
		cap.Annotations = make(map[string]string)
		// annotations is a list of tuples, each tuple is 16 bytes (ptr, len, ptr, len)
		metaData, _ := mod.Memory().Read(annoPtr, annoLen*16)
		for i := uint32(0); i < annoLen; i++ {
			kPtr := i32le.Uint32(metaData[i*16:])
			kLen := i32le.Uint32(metaData[i*16+4:])
			vPtr := i32le.Uint32(metaData[i*16+8:])
			vLen := i32le.Uint32(metaData[i*16+12:])

			k := r.readStringFromArgs(mod, kPtr, kLen)
			v := r.readStringFromArgs(mod, vPtr, vLen)
			cap.Annotations[k] = v
		}
	}

	r.logger.Info("plugin registering capability", "id", mod.Name(), "method", method, "annos", len(cap.Annotations))

	// Implementation: send a message to the command manager to update capabilities
	payload, _ := json.Marshal(cap)
	r.routerFn(ctx, api.Message{
		ID:      fmt.Sprintf("reg-cap-%d", time.Now().UnixNano()),
		Sender:  mod.Name(),
		Target:  "command-manager",
		Method:  "register-capability",
		Payload: payload,
	})
}

func (r *Runtime) internalUnregisterCapability(ctx context.Context, mod wazeroapi.Module, methodPtr, methodLen uint32) {
	method := r.readStringFromArgs(mod, methodPtr, methodLen)
	r.routerFn(ctx, api.Message{
		ID:      fmt.Sprintf("unreg-cap-%d", time.Now().UnixNano()),
		Sender:  mod.Name(),
		Target:  "command-manager",
		Method:  "unregister-capability",
		Payload: []byte(fmt.Sprintf("{\"method\":\"%s\"}", method)),
	})
}

func (r *Runtime) internalFindProviders(ctx context.Context, mod wazeroapi.Module, methodPtr, methodLen, resultPtr uint32) {
	// For now, return an empty list or implement via sync call to command manager
	mod.Memory().WriteUint32Le(resultPtr, 0)
	mod.Memory().WriteUint32Le(resultPtr+4, 0)
}

func (r *Runtime) internalGetAllCapabilities(ctx context.Context, mod wazeroapi.Module, resultPtr uint32) {
	resp, err := r.callFn(ctx, api.Message{
		ID:      fmt.Sprintf("get-caps-%d", time.Now().UnixNano()),
		Type:    api.TypeRequest,
		Sender:  mod.Name(),
		Target:  "command-manager",
		Method:  "list",
		Payload: []byte("{}"),
	})

	if err != nil {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		mod.Memory().WriteUint32Le(resultPtr+4, 0)
		return
	}

	var data struct {
		Targets []api.Registration `json:"targets"`
	}
	if err := json.Unmarshal(resp.Payload, &data); err != nil {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		mod.Memory().WriteUint32Le(resultPtr+4, 0)
		return
	}

	var allCaps []api.Capability
	for _, target := range data.Targets {
		allCaps = append(allCaps, target.Capabilities...)
	}

	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		return
	}

	// alloy_capability_t is 40 bytes
	res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(allCaps)*40))
	basePtr := uint32(res[0])
	for i, cap := range allCaps {
		r.writeCapability(ctx, mod, basePtr+uint32(i*40), cap)
	}

	mod.Memory().WriteUint32Le(resultPtr, basePtr)
	mod.Memory().WriteUint32Le(resultPtr+4, uint32(len(allCaps)))
}

func (r *Runtime) writeCapability(ctx context.Context, mod wazeroapi.Module, ptr uint32, cap api.Capability) {
	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		return
	}

	writeStr := func(fieldPtr uint32, s string) {
		if s == "" {
			mod.Memory().WriteUint32Le(fieldPtr, 0)
			mod.Memory().WriteUint32Le(fieldPtr+4, 0)
			return
		}
		res, err := alloc.Call(ctx, 0, 0, 1, uint64(len(s)))
		if err != nil || len(res) == 0 {
			return
		}
		mod.Memory().Write(uint32(res[0]), []byte(s))
		mod.Memory().WriteUint32Le(fieldPtr, uint32(res[0]))
		mod.Memory().WriteUint32Le(fieldPtr+4, uint32(len(s)))
	}

	writeStr(ptr, cap.Method)        // offset 0
	writeStr(ptr+8, cap.Description) // offset 8

	// offset 16: shortcut (option<string>)
	if cap.Shortcut != "" {
		mod.Memory().WriteUint32Le(ptr+16, 1)
		writeStr(ptr+20, cap.Shortcut)
	} else {
		mod.Memory().WriteUint32Le(ptr+16, 0)
		mod.Memory().WriteUint32Le(ptr+20, 0)
		mod.Memory().WriteUint32Le(ptr+24, 0)
	}

	// offset 28: annotations (option<list<tuple<string, string>>>)
	if len(cap.Annotations) > 0 {
		mod.Memory().WriteUint32Le(ptr+28, 1) // is_some

		metaData := make([]byte, len(cap.Annotations)*16)
		i := 0
		for k, v := range cap.Annotations {
			// Write key
			keyRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(k)))
			mod.Memory().Write(uint32(keyRes[0]), []byte(k))
			i32le.PutUint32(metaData[i*16:], uint32(keyRes[0]))
			i32le.PutUint32(metaData[i*16+4:], uint32(len(k)))

			// Write value
			valRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(v)))
			mod.Memory().Write(uint32(valRes[0]), []byte(v))
			i32le.PutUint32(metaData[i*16+8:], uint32(valRes[0]))
			i32le.PutUint32(metaData[i*16+12:], uint32(len(v)))
			i++
		}
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(metaData)))
		mod.Memory().Write(uint32(res[0]), metaData)
		mod.Memory().WriteUint32Le(ptr+32, uint32(res[0]))
		mod.Memory().WriteUint32Le(ptr+36, uint32(len(cap.Annotations)))
	} else {
		mod.Memory().WriteUint32Le(ptr+28, 0)
		mod.Memory().WriteUint32Le(ptr+32, 0)
		mod.Memory().WriteUint32Le(ptr+36, 0)
	}
}

func (r *Runtime) internalReadBuffer(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, resultPtr uint32) {
	id := r.readStringFromArgs(mod, idPtr, idLen)

	// Try Host Registry First
	if r.buffers != nil {
		if b, ok := r.buffers.GetBuffer(id); ok {
			alloc := mod.ExportedFunction("cabi_realloc")
			writeStr := func(ptr uint32, s string) {
				res, err := alloc.Call(ctx, 0, 0, 1, uint64(len(s)))
				if err != nil || len(res) == 0 {
					return
				}
				mod.Memory().Write(uint32(res[0]), []byte(s))
				mod.Memory().WriteUint32Le(ptr, uint32(res[0]))
				mod.Memory().WriteUint32Le(ptr+4, uint32(len(s)))
			}

			mod.Memory().WriteUint32Le(resultPtr, 1) // Some
			bufPtr := resultPtr + 8

			writeStr(bufPtr, b.GetID())
			writeStr(bufPtr+8, b.GetName())

			// Content list
			data := b.GetData()
			cRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(data)))
			mod.Memory().Write(uint32(cRes[0]), data)
			mod.Memory().WriteUint32Le(bufPtr+16, uint32(cRes[0]))
			mod.Memory().WriteUint32Le(bufPtr+20, uint32(len(data)))

			mod.Memory().WriteUint64Le(bufPtr+24, uint64(b.GetLastModified()))
			// Default Mime-type
			writeStr(bufPtr+32, "application/octet-stream")
			return
		}
	}

	// Synchronous call to buffer plugin (Standard Path Fallback)
	resp, err := r.callFn(ctx, api.Message{
		ID:      fmt.Sprintf("read-buf-%d", time.Now().UnixNano()),
		Type:    api.TypeRequest,
		Sender:  mod.Name(),
		Target:  "buffer",
		Method:  "read",
		Payload: []byte(fmt.Sprintf("{\"id\":\"%s\"}", id)),
	})

	if err != nil || len(resp.Payload) == 0 {
		mod.Memory().WriteUint32Le(resultPtr, 0) // None
		return
	}

	var buf struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Content      []byte `json:"content"`
		LastModified uint64 `json:"last_modified"`
		MimeType     string `json:"mime_type"`
	}
	if err := json.Unmarshal(resp.Payload, &buf); err != nil {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		return
	}

	alloc := mod.ExportedFunction("cabi_realloc")
	writeStr := func(ptr uint32, s string) {
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(s)))
		mod.Memory().Write(uint32(res[0]), []byte(s))
		mod.Memory().WriteUint32Le(ptr, uint32(res[0]))
		mod.Memory().WriteUint32Le(ptr+4, uint32(len(s)))
	}

	mod.Memory().WriteUint32Le(resultPtr, 1) // Some
	// alloy_buffer_t layout: id(8), name(8), content(8), last_modified(8), mime_type(8) = 40 bytes
	bufPtr := resultPtr + 8 // Alignment/offset check

	writeStr(bufPtr, buf.ID)
	writeStr(bufPtr+8, buf.Name)

	// Content list
	cRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(buf.Content)))
	mod.Memory().Write(uint32(cRes[0]), buf.Content)
	mod.Memory().WriteUint32Le(bufPtr+16, uint32(cRes[0]))
	mod.Memory().WriteUint32Le(bufPtr+20, uint32(len(buf.Content)))

	mod.Memory().WriteUint64Le(bufPtr+24, buf.LastModified)
	writeStr(bufPtr+32, buf.MimeType)
}

func (r *Runtime) internalWriteBuffer(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, contentPtr, contentLen uint32) uint32 {
	id := r.readStringFromArgs(mod, idPtr, idLen)
	content, _ := mod.Memory().Read(contentPtr, contentLen)

	// TRY Host Path first
	if r.buffers != nil {
		if mod.Name() == "buffer" {
			// Special: the 'buffer' plugin is the authoritative source for these
			b, err := r.buffers.CreateBuffer(id, id, int(contentLen))
			if err == nil {
				// Ensure host-side buffer is large enough for the authoritative state
				if int(contentLen) > b.GetSize() {
					_ = b.Resize(int(contentLen))
				}
				bData := b.GetData()
				copy(bData, content)
				b.Lock()
				b.Unlock()
				return 1
			}
		} else {
			// OTHER plugins can write via the buffer manager too if it's already there
			if b, ok := r.buffers.GetBuffer(id); ok {
				// Don't resize for non-authoritative plugins in this simple model,
				// just copy what fits or return error if we had one.
				bData := b.GetData()
				copy(bData, content)
				b.Lock()
				b.Unlock()
				return 1
			}
		}
	}

	payload, _ := json.Marshal(map[string]any{
		"id":      id,
		"content": content,
	})

	_, err := r.callFn(ctx, api.Message{
		ID:      fmt.Sprintf("write-buf-%d", time.Now().UnixNano()),
		Type:    api.TypeRequest,
		Sender:  mod.Name(),
		Target:  "buffer",
		Method:  "write",
		Payload: payload,
	})

	if err != nil {
		return 0
	}
	return 1
}

func (r *Runtime) internalListBuffers(ctx context.Context, mod wazeroapi.Module, resultPtr uint32) {
	resp, err := r.callFn(ctx, api.Message{
		ID:      fmt.Sprintf("list-bufs-%d", time.Now().UnixNano()),
		Type:    api.TypeRequest,
		Sender:  mod.Name(),
		Target:  "buffer",
		Method:  "list",
		Payload: []byte("{}"),
	})

	if err != nil {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		mod.Memory().WriteUint32Le(resultPtr+4, 0)
		return
	}

	var ids []string
	json.Unmarshal(resp.Payload, &ids)

	alloc := mod.ExportedFunction("cabi_realloc")
	res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(ids)*8))
	basePtr := uint32(res[0])

	for i, id := range ids {
		sRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(id)))
		mod.Memory().Write(uint32(sRes[0]), []byte(id))
		mod.Memory().WriteUint32Le(basePtr+uint32(i*8), uint32(sRes[0]))
		mod.Memory().WriteUint32Le(basePtr+uint32(i*8+4), uint32(len(id)))
	}

	mod.Memory().WriteUint32Le(resultPtr, basePtr)
	mod.Memory().WriteUint32Le(resultPtr+4, uint32(len(ids)))
}

func (r *Runtime) internalRegisterWidget(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, titlePtr, titleLen, typePtr, typeLen, contentPtr, contentLen, interval uint32) {
	w := api.Widget{
		ID:                r.readStringFromArgs(mod, idPtr, idLen),
		Title:             r.readStringFromArgs(mod, titlePtr, titleLen),
		ContentType:       r.readStringFromArgs(mod, typePtr, typeLen),
		RefreshIntervalMs: interval,
	}
	if contentLen > 0 {
		w.Content, _ = mod.Memory().Read(contentPtr, contentLen)
	}

	r.logger.Info("plugin registering dashboard widget", "plugin", mod.Name(), "widget", w.ID, "title", w.Title)

	r.mu.Lock()
	r.widgets[w.ID] = w
	r.mu.Unlock()
	r.saveWidgets()

	// Broadast as event
	wData, _ := json.Marshal(w)
	payload, _ := json.Marshal(map[string]any{
		"topic": "dashboard:widget-registered",
		"data":  json.RawMessage(wData),
	})

	r.routerFn(ctx, api.Message{
		ID:      fmt.Sprintf("reg-widget-%d", time.Now().UnixNano()),
		Type:    api.TypeEvent,
		Sender:  mod.Name(),
		Target:  "events",
		Method:  "publish",
		Payload: payload,
	})
}

func (r *Runtime) internalUnregisterWidget(ctx context.Context, mod wazeroapi.Module, idPtr, idLen uint32) {
	id := r.readStringFromArgs(mod, idPtr, idLen)
	r.logger.Info("plugin unregistering dashboard widget", "plugin", mod.Name(), "widget", id)

	r.mu.Lock()
	delete(r.widgets, id)
	r.mu.Unlock()
	r.saveWidgets()

	payload, _ := json.Marshal(map[string]any{
		"topic": "dashboard:widget-unregistered",
		"data":  map[string]string{"id": id},
	})

	r.routerFn(ctx, api.Message{
		ID:      fmt.Sprintf("unreg-widget-%d", time.Now().UnixNano()),
		Type:    api.TypeEvent,
		Sender:  mod.Name(),
		Target:  "events",
		Method:  "publish",
		Payload: payload,
	})
}

func (r *Runtime) internalUpdateWidget(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, contentPtr, contentLen uint32) {
	id := r.readStringFromArgs(mod, idPtr, idLen)
	content, _ := mod.Memory().Read(contentPtr, contentLen)

	r.mu.Lock()
	if w, ok := r.widgets[id]; ok {
		w.Content = content
		r.widgets[id] = w
	}
	r.mu.Unlock()
	r.saveWidgets()

	// Ensure the content is valid JSON (wrap in quotes if it's text)
	// Actually, let's just marshal it as a byte slice to be safe.
	payload, _ := json.Marshal(map[string]any{
		"topic": "dashboard:widget-updated",
		"data":  content,
	})

	r.routerFn(ctx, api.Message{
		ID:      fmt.Sprintf("upd-widget-%d", time.Now().UnixNano()),
		Type:    api.TypeEvent,
		Sender:  mod.Name(),
		Target:  "events",
		Method:  "publish",
		Payload: payload,
		Metadata: map[string]any{
			"widget_id": id,
		},
	})
}

// Public Workspace Management

func (r *Runtime) RegisterWorkspace(ws api.Workspace) {
	r.mu.Lock()
	r.workspaces[ws.ID] = ws
	if r.activeWorkspace == "" {
		r.activeWorkspace = ws.ID
	}
	r.mu.Unlock()
	r.saveWorkspaces()
}

func (r *Runtime) UnregisterWorkspace(id string) {
	r.mu.Lock()
	delete(r.workspaces, id)
	if r.activeWorkspace == id {
		r.activeWorkspace = ""
		for nextID := range r.workspaces {
			r.activeWorkspace = nextID
			break
		}
	}
	r.mu.Unlock()
	r.saveWorkspaces()
}

func (r *Runtime) SetActiveWorkspace(id string) {
	r.mu.Lock()
	r.activeWorkspace = id
	r.mu.Unlock()
	r.saveWorkspaces()
}

func (r *Runtime) GetActiveWorkspace() (api.Workspace, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ws, ok := r.workspaces[r.activeWorkspace]
	return ws, ok
}

func (r *Runtime) ListWorkspaces() []api.Workspace {
	r.mu.RLock()
	defer r.mu.RUnlock()
	workspaces := make([]api.Workspace, 0, len(r.workspaces))
	for _, ws := range r.workspaces {
		workspaces = append(workspaces, ws)
	}
	return workspaces
}

func (r *Runtime) ListWidgets() []api.Widget {
	r.mu.RLock()
	defer r.mu.RUnlock()
	widgets := make([]api.Widget, 0, len(r.widgets))
	for _, w := range r.widgets {
		widgets = append(widgets, w)
	}
	return widgets
}

func (r *Runtime) internalGetBufferView(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, resultPtr uint32) {
	id := r.readStringFromArgs(mod, idPtr, idLen)
	pluginID := mod.Name()

	r.logger.Debug("get-buffer-view called", "plugin", pluginID, "id", id)

	if r.buffers != nil {
		if b, ok := r.buffers.GetBuffer(id); ok {
			alloc := mod.ExportedFunction("cabi_realloc")
			data := b.GetData()
			size := uint32(len(data))

			// Check if we already have a view
			r.mu.Lock()
			if pluginViews, ok := r.bufferViews[pluginID]; ok {
				if ptr, ok := pluginViews[id]; ok {
					r.mu.Unlock()
					// Already mapped
					mod.Memory().WriteUint32Le(resultPtr, 1) // Some
					mod.Memory().WriteUint32Le(resultPtr+4, ptr)
					mod.Memory().WriteUint32Le(resultPtr+8, size)
					return
				}
			} else {
				r.bufferViews[pluginID] = make(map[string]uint32)
			}
			r.mu.Unlock()

			// Register a watcher for this buffer to sync it to THIS plugin's view
			b.OnUpdate(func(updatedID string, offset, length int) {
				if updatedID != id {
					return
				}

				r.mu.RLock()
				instance, ok := r.plugins[pluginID]
				pluginViews := r.bufferViews[pluginID]
				r.mu.RUnlock()

				if !ok || instance.mod == nil {
					return
				}

				guestPtr, ok := pluginViews[id]
				if !ok {
					return
				}

				// Fetch fresh data from host buffer
				hostData := b.GetData()
				if offset+length > len(hostData) {
					length = len(hostData) - offset
				}

				if offset < len(hostData) && length > 0 {
					// Sync to guest memory
					instance.mod.Memory().Write(guestPtr+uint32(offset), hostData[offset:offset+length])

					// Optional: Notify guest of update if it exports 'on_buffer_update'
					if onUpdate := instance.mod.ExportedFunction("on_buffer_update"); onUpdate != nil {
						// Using context.Background() as we don't want to block the watcher too long
						_, _ = onUpdate.Call(context.Background(), uint64(guestPtr), uint64(offset), uint64(length))
					}
				}
			})

			// Initial allocation in guest
			res, err := alloc.Call(ctx, 0, 0, 1, uint64(size))
			if err == nil {
				guestPtr := uint32(res[0])
				// Initial copy into guest space
				mod.Memory().Write(guestPtr, data)

				r.mu.Lock()
				r.bufferViews[pluginID][id] = guestPtr
				r.mu.Unlock()

				// Return the (pointer, size) back to guest
				mod.Memory().WriteUint32Le(resultPtr, 1) // Some
				mod.Memory().WriteUint32Le(resultPtr+4, guestPtr)
				mod.Memory().WriteUint32Le(resultPtr+8, size)
				return
			}
		}
	}

	// Stub: fallback or not found
	mod.Memory().WriteUint32Le(resultPtr, 0) // None
}
