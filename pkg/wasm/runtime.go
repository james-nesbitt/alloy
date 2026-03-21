package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
	"github.com/tetratelabs/wazero"
	wazeroapi "github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// RouterFunc is a function that routes messages to their targets.
type RouterFunc func(ctx context.Context, msg api.Message)

// CallFunc is a function that performs a synchronous call to another plugin.
type CallFunc func(ctx context.Context, msg api.Message) (api.Message, error)

// Runtime manages the WASM runtime environment.
type Runtime struct {
	r          wazero.Runtime
	logger     *slog.Logger
	kv         storage.StateStore
	dataDir    string
	routerFn   RouterFunc
	callFn     CallFunc
	
	// Map to look up active plugin instances by name for host callbacks
	mu      sync.RWMutex
	plugins map[string]*Instance
}

// NewRuntime creates a new WASM runtime.
func NewRuntime(ctx context.Context, logger *slog.Logger, kv storage.StateStore, dataDir string, router RouterFunc, call CallFunc) (*Runtime, error) {
	r := wazero.NewRuntime(ctx)
	
	rt := &Runtime{
		r:        r,
		logger:   logger,
		kv:       kv,
		dataDir:  dataDir,
		routerFn: router,
		callFn:   call,
		plugins:  make(map[string]*Instance),
	}

	return rt, nil
}

// LoadPlugin instantiates a WASM plugin.
func (r *Runtime) LoadPlugin(ctx context.Context, id string, wasmBytes []byte, fuelLimit uint64, memoryLimit Pages, caps []api.Capability) (*Instance, error) {
	config := wazero.NewModuleConfig().
		WithName(id).
		WithStdout(r.newLoggerWriter(id, "stdout")).
		WithStderr(r.newLoggerWriter(id, "stderr")).
		WithFS(os.DirFS(".")). // default, replaced if dataDir set
		WithStartFunctions()

	// Map storage for the plugin
	if r.dataDir != "" {
		pluginDir := filepath.Join(r.dataDir, id)
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create plugin storage dir: %w", err)
		}
		config = config.WithFS(os.DirFS(pluginDir))
	}

	// Compile the module
	compiled, err := r.r.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to compile module %s: %w", id, err)
	}

	startChan := make(chan [2]uint32, 1)
	instCtx, instCancel := context.WithCancel(context.Background())
	loadCtx := context.WithValue(instCtx, "alloy.start_chan", startChan)

	// Instantiate the module
	mod, err := r.r.InstantiateModule(loadCtx, compiled, config)
	if err != nil {
		instCancel()
		return nil, fmt.Errorf("failed to instantiate module %s: %w", id, err)
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
		defaultFuel:  fuelLimit,
		status:       StatusRunning,
		capabilities: caps,
	}

	// Register it so host functions can find it
	r.mu.Lock()
	r.plugins[id] = instance
	r.mu.Unlock()

	// Wait for readiness handshake if it was a command module
	if startFunc := mod.ExportedFunction("_start"); startFunc != nil {
		// We use a background context for the long-running _start function
		// but we still want the startChan from loadCtx.
		startCtx := context.WithValue(context.Background(), "alloy.start_chan", startChan)
		go func() {
			if _, err := startFunc.Call(startCtx); err != nil {
				r.logger.Error("plugin _start exited with error", "id", id, "error", err)
				instance.status = StatusCrashed
				instance.lastError = err.Error()
			}
		}()

		// Wait for the plugin to signal it's started
		select {
		case ptrs := <-startChan:
			r.logger.Info("wasm plugin ready", "id", id, "in_ptr", ptrs[0], "out_ptr", ptrs[1])
			instance.inPtr = ptrs[0]
			instance.outPtr = ptrs[1]
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for plugin %s to start", id)
		}
	}

	return instance, nil
}

// Close shuts down the runtime.
func (r *Runtime) Close(ctx context.Context) error {
	return r.r.Close(ctx)
}

// InstantiateAlloyHost defines the host-side functions available to plugins.
func (r *Runtime) InstantiateAlloyHost(ctx context.Context) (wazeroapi.Module, error) {
	_, _ = wasi_snapshot_preview1.Instantiate(ctx, r.r)

	r.logger.Info("Instantiating alloy host module")
	return r.r.NewHostModuleBuilder("alloy").
		NewFunctionBuilder().
		WithFunc(r.alloyLog).
		Export("log").
		NewFunctionBuilder().
		WithFunc(r.alloyKVSet).
		Export("kv_set").
		NewFunctionBuilder().
		WithFunc(r.alloyKVGet).
		Export("kv_get").
		NewFunctionBuilder().
		WithFunc(r.alloyKVDelete).
		Export("kv_delete").
		NewFunctionBuilder().
		WithFunc(r.alloyKVList).
		Export("kv_list").
		NewFunctionBuilder().
		WithFunc(r.alloyRouteMessage).
		Export("route_message").
		NewFunctionBuilder().
		WithFunc(r.alloyStarted).
		Export("started").
		NewFunctionBuilder().
		WithFunc(r.hostYield).
		Export("yield").
		NewFunctionBuilder().
		WithFunc(r.hostSleepForever).
		Export("sleep_forever").
		NewFunctionBuilder().
		WithFunc(r.hostGetNextMessage).
		Export("get_next_message").
		NewFunctionBuilder().
		WithFunc(r.alloySendResponse).
		Export("send_response").
		NewFunctionBuilder().
		WithFunc(r.hostFetch).
		Export("fetch").
		NewFunctionBuilder().
		WithFunc(r.alloyCall).
		Export("call").
		Instantiate(ctx)
}

func (r *Runtime) hostYield(ctx context.Context, mod wazeroapi.Module) {}

func (r *Runtime) hostSleepForever(ctx context.Context, mod wazeroapi.Module) {
	// Effectively block the main goroutine
	<-ctx.Done()
}

func (r *Runtime) hostGetNextMessage(ctx context.Context, mod wazeroapi.Module, ptr, maxSize uint32) uint32 {
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()

	if !ok { return 0 }

	select {
	case msg := <-instance.msgChan:
		r.logger.Debug("pulling message for guest", "plugin", mod.Name(), "method", msg.Method, "id", msg.ID)
		data, err := json.Marshal(msg)
		if err != nil { return 0 }
		if uint32(len(data)) > maxSize { return 0 }
		if mod.Memory().Write(ptr, data) {
			return uint32(len(data))
		}
		return 0
	case <-ctx.Done():
		return 0
	}
}

func (r *Runtime) alloySendResponse(ctx context.Context, mod wazeroapi.Module, ptr, size uint32) {
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()

	if !ok { return }

	if size == 0 {
		select {
		case instance.respChan <- api.Message{}:
		default:
		}
		return
	}

	buf, ok := mod.Memory().Read(ptr, size)
	if !ok { return }

	var resp api.Message
	if err := json.Unmarshal(buf, &resp); err != nil {
		r.logger.Error("failed to unmarshal guest response", "plugin", mod.Name(), "error", err)
		return
	}

	r.logger.Debug("received response from guest", "plugin", mod.Name(), "id", resp.ID)
	select {
	case instance.respChan <- resp:
	default:
	}
}

func (r *Runtime) alloyStarted(ctx context.Context, mod wazeroapi.Module, inPtr, outPtr uint32) {
	r.logger.Info("wasm plugin signaled started (host)", "id", mod.Name())
	if router, ok := ctx.Value("alloy.start_chan").(chan [2]uint32); ok {
		select {
		case router <- [2]uint32{inPtr, outPtr}:
		default:
		}
	}
}

func (r *Runtime) alloyLog(ctx context.Context, mod wazeroapi.Module, ptr, size uint32) {
	buf, ok := mod.Memory().Read(ptr, size)
	if !ok { return }
	r.logger.Info("wasm_log", "plugin", mod.Name(), "msg", string(buf))
}

func (r *Runtime) alloyKVSet(ctx context.Context, mod wazeroapi.Module, kPtr, kLen, vPtr, vLen uint32) uint32 {
	key, ok := mod.Memory().Read(kPtr, kLen)
	if !ok { return 1 }
	
	var val []byte
	if vLen > 0 {
		val, ok = mod.Memory().Read(vPtr, vLen)
		if !ok { return 1 }
	}

	if err := r.kv.Set(mod.Name(), string(key), val); err != nil {
		return 1
	}
	return 0
}

func (r *Runtime) alloyKVGet(ctx context.Context, mod wazeroapi.Module, kPtr, kLen, vPtr, vMaxLen uint32) uint32 {
	key, ok := mod.Memory().Read(kPtr, kLen)
	if !ok { return 0 }

	val, err := r.kv.Get(mod.Name(), string(key))
	if err != nil {
		return 0
	}

	if vMaxLen == 0 {
		return uint32(len(val))
	}

	if uint32(len(val)) > vMaxLen {
		return 0
	}

	if mod.Memory().Write(vPtr, val) {
		return uint32(len(val))
	}
	return 0
}

func (r *Runtime) alloyKVDelete(ctx context.Context, mod wazeroapi.Module, kPtr, kLen uint32) uint32 {
	key, ok := mod.Memory().Read(kPtr, kLen)
	if !ok { return 1 }
	if err := r.kv.Delete(mod.Name(), string(key)); err != nil {
		return 1
	}
	return 0
}

func (r *Runtime) alloyKVList(ctx context.Context, mod wazeroapi.Module, pPtr, pLen, respPtrPtr, respSizePtr uint32) uint32 {
	prefix, ok := mod.Memory().Read(pPtr, pLen)
	if !ok { return 1 }

	keys, err := r.kv.List(mod.Name(), string(prefix))
	if err != nil { return 1 }

	data, err := json.Marshal(keys)
	if err != nil { return 1 }

	malloc := mod.ExportedFunction("alloy_malloc")
	if malloc == nil { return 1 }
	res, err := malloc.Call(ctx, uint64(len(data)))
	if err != nil || len(res) == 0 { return 1 }
	ptr := uint32(res[0])

	if !mod.Memory().Write(ptr, data) { return 1 }
	if !mod.Memory().WriteUint32Le(respPtrPtr, ptr) { return 1 }
	if !mod.Memory().WriteUint32Le(respSizePtr, uint32(len(data))) { return 1 }
	return 0
}

func (r *Runtime) alloyRouteMessage(ctx context.Context, mod wazeroapi.Module, ptr, size uint32) {
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()

	buf, ok_mem := mod.Memory().Read(ptr, size)
	if !ok_mem { return }
	var msg api.Message
	if err := json.Unmarshal(buf, &msg); err != nil { return }
	
	// Use instance context if available, otherwise wazero ctx
	routeCtx := ctx
	if ok {
		routeCtx = instance.ctx
	}
	r.routerFn(routeCtx, msg)
}

func (r *Runtime) alloyCall(ctx context.Context, mod wazeroapi.Module, ptr, size, respPtrPtr, respSizePtr uint32) uint32 {
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()

	buf, ok_mem := mod.Memory().Read(ptr, size)
	if !ok_mem { return 1 }
	var msg api.Message
	if err := json.Unmarshal(buf, &msg); err != nil { return 1 }

	// Use instance context if available
	callCtx := ctx
	if ok {
		callCtx = instance.ctx
	}
	resp, err := r.callFn(callCtx, msg)
	if err != nil { return 1 }

	data, err := json.Marshal(resp)
	if err != nil { return 1 }

	malloc := mod.ExportedFunction("alloy_malloc")
	if malloc == nil { return 1 }
	res, err := malloc.Call(ctx, uint64(len(data)))
	if err != nil || len(res) == 0 { return 1 }
	ptrResp := uint32(res[0])

	if !mod.Memory().Write(ptrResp, data) { return 1 }
	if !mod.Memory().WriteUint32Le(respPtrPtr, ptrResp) { return 1 }
	if !mod.Memory().WriteUint32Le(respSizePtr, uint32(len(data))) { return 1 }
	return 0
}

func (r *Runtime) hostFetch(ctx context.Context, mod wazeroapi.Module, reqPtr, reqSize, respPtrPtr, respSizePtr uint32) uint32 {
	// Not implemented yet
	return 1
}

type loggerWriter struct {
	logger *slog.Logger
	id     string
	stream string
	buf    []byte
}

func (r *Runtime) newLoggerWriter(id, stream string) *loggerWriter {
	return &loggerWriter{
		logger: r.logger,
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
		if idx == -1 { break }
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

type Pages uint32
