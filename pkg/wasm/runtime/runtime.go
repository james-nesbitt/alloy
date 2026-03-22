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
	capabilities []api.Capability
	status       Status
	metadata     api.PluginMetadata

	startedCh chan struct{}

	// pending responses: msgID -> channel
	pmu     sync.Mutex
	pending map[string]chan api.Message
}

// Metadata returns the plugin's metadata.
func (i *Instance) Metadata() api.PluginMetadata {
	return i.metadata
}

// Close closes the plugin instance.
func (i *Instance) Close(ctx context.Context) error {
	i.cancel()
	if i.mod != nil {
		return i.mod.Close(ctx)
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
	router func(ctx context.Context, msg api.Message),
	call func(ctx context.Context, msg api.Message) (api.Message, error),
) (*Runtime, error) {
	r := wazero.NewRuntime(ctx)
	logger.Info("creating new WIT-based runtime (v2.9-async-compile)")

	rt := &Runtime{
		runtime:  r,
		logger:   logger,
		kv:       kv,
		dataDir:  dataDir,
		routerFn: router,
		callFn:   call,
		plugins:  make(map[string]*Instance),
	}

	// Instantiate the host module with functions
	hostMod, err := rt.instantiateHostModule(ctx)
	if err != nil {
		return nil, err
	}
	rt.hostModule = hostMod

	// Instantiate WASI
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		return nil, fmt.Errorf("failed to instantiate WASI: %w", err)
	}

	// Instantiate asyncify (dummy)
	_, _ = r.NewHostModuleBuilder("asyncify").
		NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start").
		NewFunctionBuilder().WithFunc(func() {}).Export("stop").
		NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start_unwind").
		NewFunctionBuilder().WithFunc(func() {}).Export("stop_unwind").
		NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start_rewind").
		NewFunctionBuilder().WithFunc(func() {}).Export("stop_rewind").
		Instantiate(ctx)

	return rt, nil
}

// instantiateHostModule creates the host module with WIT functions.
func (r *Runtime) instantiateHostModule(ctx context.Context) (wazeroapi.Module, error) {
	builder := r.runtime.NewHostModuleBuilder("alloy")

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
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(s)))
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
	msg := r.readMessageFromArgs(mod, idPtr, idLen, typePtr, typeLen, methodPtr, methodLen, senderPtr, senderLen, targetSet, targetPtr, targetLen, payloadPtr, payloadLen, timestamp)
	r.routerFn(ctx, msg)
}

func (r *Runtime) internalCall(
	ctx context.Context, mod wazeroapi.Module,
	idPtr, idLen, typePtr, typeLen, methodPtr, methodLen, senderPtr, senderLen uint32,
	targetSet, targetPtr, targetLen, payloadPtr, payloadLen uint32,
	timestamp int64, resultPtr uint32,
) {
	apiMsg := r.readMessageFromArgs(mod, idPtr, idLen, typePtr, typeLen, methodPtr, methodLen, senderPtr, senderLen, targetSet, targetPtr, targetLen, payloadPtr, payloadLen, timestamp)
	resp, err := r.callFn(ctx, apiMsg)
	if err != nil {
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
	fuelLimit uint64,
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
	}

	r.mu.Lock()
	r.plugins[id] = instance
	r.mu.Unlock()

	r.logger.Debug("created new plugin instance", "id", id, "ptr", fmt.Sprintf("%p", instance))

	// Compilation and Instantion happen in background to avoid blocking other plugins or host boot
	go func() {
		r.logger.Debug("compiling wasm module", "id", id, "bytes", len(wasmBytes))
		compiled, err := r.runtime.CompileModule(instCtx, wasmBytes)
		if err != nil {
			r.logger.Error("failed to compile module", "id", id, "error", err)
			instCancel()
			return
		}

		r.logger.Debug("instantiating wasm module", "id", id)

		config := wazero.NewModuleConfig().
			WithName(id).
			WithStdout(newLoggerWriter(r.logger, id, "stdout")).
			WithStderr(newLoggerWriter(r.logger, id, "stderr")).
			WithFS(os.DirFS(pluginDir))

		mod, err := r.runtime.InstantiateModule(instCtx, compiled, config)
		if err != nil {
			r.logger.Error("failed to instantiate module", "id", id, "error", err)
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
		instance.cancel()
		if instance.mod != nil {
			_ = instance.mod.Close(ctx)
		}
	}

	return r.runtime.Close(ctx)
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
