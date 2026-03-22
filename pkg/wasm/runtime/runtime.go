package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
	"github.com/tetratelabs/wazero"
	wazeroapi "github.com/tetratelabs/wazero/api"
)

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
	respChan     chan api.Message
	capabilities []api.Capability
	status       Status
	metadata     api.PluginMetadata
}

// Status represents the plugin's execution status.
type Status int

const (
	StatusRunning Status = iota
	StatusPaused
	StatusStopped
	StatusCrashed
)

// Protocol types for WIT communication (matching alloy.wit)
type witMessage struct {
	Id        string            `json:"id"`
	Method    string            `json:"method"`
	Sender    string            `json:"sender"`
	Target    witOption[string] `json:"target"`
	Payload   []byte            `json:"payload"`
	Timestamp uint64            `json:"timestamp"`
}

type witOption[T any] struct {
	Value T    `json:"value,omitempty"`
	Set   bool `json:"set"`
}

type witCapability struct {
	Method      string `json:"method"`
	Description string `json:"description"`
}

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

	return rt, nil
}

// instantiateHostModule creates the host module with WIT functions.
func (r *Runtime) instantiateHostModule(ctx context.Context) (wazeroapi.Module, error) {
	builder := r.runtime.NewHostModuleBuilder("alloy")
	builder = r.registerWITFunctions(builder)
	return builder.Instantiate(ctx)
}

func (r *Runtime) registerWITFunctions(builder wazero.HostModuleBuilder) wazero.HostModuleBuilder {
	return builder.
		NewFunctionBuilder().WithFunc(r.witHandleMessage).Export("handle-message").
		NewFunctionBuilder().WithFunc(r.witRouteMessage).Export("route-message").
		NewFunctionBuilder().WithFunc(r.witCall).Export("call").
		NewFunctionBuilder().WithFunc(r.witGetNextMessage).Export("get-next-message").
		NewFunctionBuilder().WithFunc(r.witSendResponse).Export("send-response").
		NewFunctionBuilder().WithFunc(r.witLog).Export("log").
		NewFunctionBuilder().WithFunc(r.witKVSet).Export("kv-set").
		NewFunctionBuilder().WithFunc(r.witKVGet).Export("kv-get").
		NewFunctionBuilder().WithFunc(r.witKVDelete).Export("kv-delete").
		NewFunctionBuilder().WithFunc(r.witKVList).Export("kv-list").
		NewFunctionBuilder().WithFunc(r.witInit).Export("init").
		NewFunctionBuilder().WithFunc(r.witStarted).Export("started")
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

	config := wazero.NewModuleConfig().
		WithName(id).
		WithStdout(newLoggerWriter(r.logger, id, "stdout")).
		WithStderr(newLoggerWriter(r.logger, id, "stderr")).
		WithFS(os.DirFS(pluginDir))

	compiled, err := r.runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to compile module %s: %w", id, err)
	}

	instCtx, instCancel := context.WithCancel(context.Background())
	mod, err := r.runtime.InstantiateModule(instCtx, compiled, config)
	if err != nil {
		instCancel()
		return nil, fmt.Errorf("failed to instantiate module %s: %w", id, err)
	}

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
		metadata: api.PluginMetadata{
			ID:           id,
			Capabilities: caps,
		},
	}

	r.mu.Lock()
	r.plugins[id] = instance
	r.mu.Unlock()

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
		instance.Close(ctx)
	}
	return r.runtime.Close(ctx)
}

func (i *Instance) Close(ctx context.Context) error {
	i.cancel()
	if i.mod != nil {
		return i.mod.Close(ctx)
	}
	return nil
}

// Metadata returns the plugin metadata.
func (i *Instance) Metadata() api.PluginMetadata {
	return i.metadata
}

// WIT Host Function Implementations

func (r *Runtime) witLog(ctx context.Context, mod wazeroapi.Module, levelPtr, levelLen, msgPtr, msgLen uint32) {
	levelData, _ := mod.Memory().Read(levelPtr, levelLen)
	msgData, _ := mod.Memory().Read(msgPtr, msgLen)
	level := string(levelData)
	msg := string(msgData)
	switch level {
	case "debug":
		r.logger.Debug("wasm_log", "plugin", mod.Name(), "msg", msg)
	case "info":
		r.logger.Info("wasm_log", "plugin", mod.Name(), "msg", msg)
	case "warn":
		r.logger.Warn("wasm_log", "plugin", mod.Name(), "msg", msg)
	case "error":
		r.logger.Error("wasm_log", "plugin", mod.Name(), "msg", msg)
	default:
		r.logger.Info("wasm_log", "plugin", mod.Name(), "level", level, "msg", msg)
	}
}

func (r *Runtime) witKVSet(ctx context.Context, mod wazeroapi.Module, keyPtr, keyLen, valuePtr, valueLen uint32) uint32 {
	keyData, _ := mod.Memory().Read(keyPtr, keyLen)
	valueData, _ := mod.Memory().Read(valuePtr, valueLen)
	if err := r.kv.Set(mod.Name(), string(keyData), valueData); err != nil {
		return 1
	}
	return 0
}

func (r *Runtime) witKVGet(ctx context.Context, mod wazeroapi.Module, keyPtr, keyLen, valuePtrPtr, valueSizePtr uint32) uint32 {
	keyData, _ := mod.Memory().Read(keyPtr, keyLen)
	value, err := r.kv.Get(mod.Name(), string(keyData))
	if err != nil || value == nil {
		return 0
	}

	// Write to guest memory
	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		return 1
	}
	res, err := alloc.Call(ctx, 0, 0, 1, uint64(len(value)))
	if err != nil || len(res) == 0 {
		return 1
	}

	mod.Memory().Write(uint32(res[0]), value)
	mod.Memory().WriteUint32Le(valuePtrPtr, uint32(res[0]))
	mod.Memory().WriteUint32Le(valueSizePtr, uint32(len(value)))
	return 0
}

func (r *Runtime) witKVDelete(ctx context.Context, mod wazeroapi.Module, keyPtr, keyLen uint32) uint32 {
	keyData, _ := mod.Memory().Read(keyPtr, keyLen)
	if err := r.kv.Delete(mod.Name(), string(keyData)); err != nil {
		return 1
	}
	return 0
}

func (r *Runtime) witKVList(ctx context.Context, mod wazeroapi.Module, prefixPtr, prefixLen, keysPtrPtr, keysSizePtr uint32) uint32 {
	prefixData, _ := mod.Memory().Read(prefixPtr, prefixLen)
	keys, err := r.kv.List(mod.Name(), string(prefixData))
	if err != nil {
		return 1
	}
	data, _ := json.Marshal(keys)

	alloc := mod.ExportedFunction("cabi_realloc")
	res, err := alloc.Call(ctx, 0, 0, 1, uint64(len(data)))
	if err != nil {
		return 1
	}

	mod.Memory().Write(uint32(res[0]), data)
	mod.Memory().WriteUint32Le(keysPtrPtr, uint32(res[0]))
	mod.Memory().WriteUint32Le(keysSizePtr, uint32(len(data)))
	return 0
}

func (r *Runtime) witInit(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, capsPtr, capsLen uint32) {
	// Optional sync initialization
}

func (r *Runtime) witStarted(ctx context.Context, mod wazeroapi.Module) {
	r.logger.Info("wasm plugin ready", "plugin", mod.Name())
}

func (r *Runtime) witHandleMessage(ctx context.Context, mod wazeroapi.Module, msgPtr, msgLen uint32) uint32 {
	// Usually plugins handle messages via get-next-message
	return 0
}

func (r *Runtime) witRouteMessage(ctx context.Context, mod wazeroapi.Module, msgPtr, msgLen uint32) {
	data, _ := mod.Memory().Read(msgPtr, msgLen)
	var wmsg witMessage
	if err := json.Unmarshal(data, &wmsg); err == nil {
		apiMsg := api.Message{
			ID:      wmsg.Id,
			Method:  wmsg.Method,
			Sender:  mod.Name(),
			Payload: json.RawMessage(wmsg.Payload),
		}
		if wmsg.Target.Set {
			apiMsg.Target = wmsg.Target.Value
		}
		r.routerFn(ctx, apiMsg)
	}
}

func (r *Runtime) witCall(ctx context.Context, mod wazeroapi.Module, msgPtr, msgLen, respPtrPtr, respSizePtr uint32) uint32 {
	data, _ := mod.Memory().Read(msgPtr, msgLen)
	var wmsg witMessage
	if err := json.Unmarshal(data, &wmsg); err == nil {
		apiMsg := api.Message{
			ID:      wmsg.Id,
			Method:  wmsg.Method,
			Sender:  mod.Name(),
			Payload: json.RawMessage(wmsg.Payload),
		}
		if wmsg.Target.Set {
			apiMsg.Target = wmsg.Target.Value
		}

		resp, err := r.callFn(ctx, apiMsg)
		if err != nil {
			return 1
		}

		wresp := witMessage{
			Id: resp.ID, Method: resp.Method, Sender: resp.Sender, Payload: []byte(resp.Payload),
		}
		if resp.Target != "" {
			wresp.Target = witOption[string]{Value: resp.Target, Set: true}
		}

		respData, _ := json.Marshal(wresp)
		alloc := mod.ExportedFunction("cabi_realloc")
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(respData)))

		mod.Memory().Write(uint32(res[0]), respData)
		mod.Memory().WriteUint32Le(respPtrPtr, uint32(res[0]))
		mod.Memory().WriteUint32Le(respSizePtr, uint32(len(respData)))
		return 0
	}
	return 1
}

func (r *Runtime) witGetNextMessage(ctx context.Context, mod wazeroapi.Module, ptrPtr, sizePtr uint32) uint32 {
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()
	if !ok {
		return 0
	}

	select {
	case msg := <-instance.msgChan:
		wmsg := witMessage{
			Id: msg.ID, Method: msg.Method, Sender: msg.Sender, Payload: []byte(msg.Payload),
		}
		if msg.Target != "" {
			wmsg.Target = witOption[string]{Value: msg.Target, Set: true}
		}
		data, _ := json.Marshal(wmsg)

		alloc := mod.ExportedFunction("cabi_realloc")
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(data)))
		mod.Memory().Write(uint32(res[0]), data)
		mod.Memory().WriteUint32Le(ptrPtr, uint32(res[0]))
		mod.Memory().WriteUint32Le(sizePtr, uint32(len(data)))
		return 1
	default:
		return 0
	}
}

func (r *Runtime) witSendResponse(ctx context.Context, mod wazeroapi.Module, msgPtr, msgLen uint32) {
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()
	if !ok {
		return
	}

	data, _ := mod.Memory().Read(msgPtr, msgLen)
	var wmsg witMessage
	if err := json.Unmarshal(data, &wmsg); err == nil {
		resp := api.Message{
			ID: wmsg.Id, Method: wmsg.Method, Sender: mod.Name(), Payload: json.RawMessage(wmsg.Payload),
		}
		select {
		case instance.respChan <- resp:
		default:
		}
	}
}

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
	default:
		return errors.New("channel full")
	}
}

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
	case <-time.After(1 * time.Second):
		return api.Message{}, errors.New("timeout")
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
