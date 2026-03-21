package wasm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
	"github.com/tetratelabs/wazero"
	wazeroapi "github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const (
	HostModuleName = "alloy"
	WasmPageSize   = 65536 // 64KB
)

// Runtime manages the wazero WASM environment.
type Runtime struct {
	logger   *slog.Logger
	r        wazero.Runtime
	kv       storage.StateStore
	dataDir  string
	routerFn func(context.Context, api.Message)
	callFn   func(context.Context, api.Message) (api.Message, error)
}

// NewRuntime creates a new Alloy WASM runtime.
func NewRuntime(ctx context.Context, logger *slog.Logger, kv storage.StateStore, dataDir string, router func(context.Context, api.Message), call func(context.Context, api.Message) (api.Message, error)) (*Runtime, error) {
	// Use Compiler for better performance.
	nc := wazero.NewRuntimeConfigCompiler().
		WithCoreFeatures(wazeroapi.CoreFeaturesV2)

	rt := wazero.NewRuntimeWithConfig(ctx, nc)

	// Ensure dataDir exists
	if dataDir != "" {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create runtime data directory: %w", err)
		}
	}

	// Instantiate WASI properly
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	// Instantiate Asyncify stub (needed by TinyGo in some configurations)
	if _, err := rt.NewHostModuleBuilder("asyncify").
		NewFunctionBuilder().WithFunc(func(i int32) {}).Export("start_unwind").
		NewFunctionBuilder().WithFunc(func() {}).Export("stop_unwind").
		NewFunctionBuilder().WithFunc(func(i int32) {}).Export("start_rewind").
		NewFunctionBuilder().WithFunc(func() {}).Export("stop_rewind").
		NewFunctionBuilder().WithFunc(func() int32 { return 0 }).Export("get_state").
		Instantiate(ctx); err != nil {
		return nil, fmt.Errorf("failed to instantiate asyncify stub: %w", err)
	}

	return &Runtime{
		logger:   logger,
		r:        rt,
		kv:       kv,
		dataDir:  dataDir,
		routerFn: router,
		callFn:   call,
	}, nil
}

func (r *Runtime) recoverPanic(id string) {
	if err := recover(); err != nil {
		r.logger.Error("WASM host-guest call panic recovered", "plugin", id, "error", err)
	}
}

type logWriter struct {
	logger *slog.Logger
	plugin string
	isErr  bool
	buf    bytes.Buffer
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	for _, b := range p {
		if b == '\n' {
			w.flush()
		} else {
			w.buf.WriteByte(b)
			if w.buf.Len() > 4096 {
				w.flush()
			}
		}
	}
	return n, nil
}

func (w *logWriter) flush() {
	msg := strings.TrimSpace(w.buf.String())
	if msg != "" {
		if w.isErr {
			w.logger.Error("wasm_stderr", "plugin", w.plugin, "msg", msg)
		} else {
			w.logger.Info("wasm_stdout", "plugin", w.plugin, "msg", msg)
		}
	}
	w.buf.Reset()
}

func (r *Runtime) LoadPlugin(ctx context.Context, id string, wasmBytes []byte, memoryLimitMB uint64, fuelLimit uint64, caps []api.Capability) (api.Plugin, error) {
	config := wazero.NewModuleConfig().
		WithName(id).
		WithStdout(&logWriter{logger: r.logger, plugin: id}).
		WithStderr(&logWriter{logger: r.logger, plugin: id, isErr: true}).
		WithSysWalltime().
		WithSysNanotime()

	if memoryLimitMB > 0 {
		// 1 MB = 1024 * 1024 / 65536 = 16 pages
		// Note: wazero v1.x does not support setting per-module memory limits via ModuleConfig.
		// These must be defined in the WASM binary or set globally for the runtime.
		// _ = uint32(memoryLimitMB * 1024 * 1024 / WasmPageSize)
	}

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
	loadCtx := context.WithValue(ctx, "alloy.start_chan", startChan)

	// Instantiate the module. 
	// We use WithStartFunctions() with no args to prevent InstantiateModule from 
	// blocking on _start if it's a command module.
	mod, err := r.r.InstantiateModule(loadCtx, compiled, config.WithStartFunctions())
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate module %s: %w", id, err)
	}

	if startFunc := mod.ExportedFunction("_start"); startFunc != nil {
		// Command mode - call _start in a goroutine
		r.logger.Info("starting wasm command goroutine", "id", id)
		go func() {
			defer r.recoverPanic(id)
			_, _ = startFunc.Call(loadCtx)
		}()
		
		// Wait for the plugin to signal it's started
		var ptrs [2]uint32
		select {
		case ptrs = <-startChan:
			r.logger.Info("wasm plugin ready", "id", id, "in_ptr", ptrs[0], "out_ptr", ptrs[1])
			// Wait a tiny bit more for TinyGo stack/scheduler to settle
			time.Sleep(5 * time.Millisecond)
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for plugin %s to start", id)
		}

		r.logger.Info("instantiated wasm module", "id", id)

		return &Instance{
			id:           id,
			mod:          mod,
			logger:       r.logger,
			inPtr:        ptrs[0],
			outPtr:       ptrs[1],
			defaultFuel:  fuelLimit,
			status:       StatusRunning,
			capabilities: caps,
		}, nil
	}

	return &Instance{
		id:           id,
		mod:          mod,
		logger:       r.logger,
		defaultFuel:  fuelLimit,
		status:       StatusRunning,
		capabilities: caps,
	}, nil
}

// Close shuts down the runtime.
func (r *Runtime) Close(ctx context.Context) error {
	return r.r.Close(ctx)
}

// InstantiateAlloyHost defines the host-side functions available to plugins.
// alloy_kv_delete(kPtr, kLen uint32) uint32
func (r *Runtime) alloyKVDelete(ctx context.Context, mod wazeroapi.Module, stack []uint64) {
	kPtr := uint32(stack[0])
	kLen := uint32(stack[1])

	mem := mod.Memory()
	keyBuf, ok := mem.Read(kPtr, kLen)
	if !ok {
		stack[0] = 1
		return
	}

	key := string(keyBuf)
	namespace := mod.Name()
	if strings.HasPrefix(key, "shared:") {
		namespace = "shared"
		key = key[7:]
	}

	if err := r.kv.Delete(namespace, key); err != nil {
		r.logger.Error("kv delete failed", "namespace", namespace, "key", key, "error", err)
		stack[0] = 1
		return
	}
	stack[0] = 0
}

// alloy_kv_list(pPtr, pLen, respPtrPtr, respSizePtr uint32) uint32
func (r *Runtime) alloyKVList(ctx context.Context, mod wazeroapi.Module, stack []uint64) {
	pPtr := uint32(stack[0])
	pLen := uint32(stack[1])
	respPtrPtr := uint32(stack[2])
	respSizePtr := uint32(stack[3])

	mem := mod.Memory()
	pBuf, ok := mem.Read(pPtr, pLen)
	if !ok {
		stack[0] = 1
		return
	}

	prefix := string(pBuf)
	namespace := mod.Name()
	if strings.HasPrefix(prefix, "shared:") {
		namespace = "shared"
		prefix = prefix[7:]
	}

	keys, err := r.kv.List(namespace, prefix)
	if err != nil {
		r.logger.Error("kv list failed", "namespace", namespace, "prefix", prefix, "error", err)
		stack[0] = 1
		return
	}

	data, err := json.Marshal(keys)
	if err != nil {
		stack[0] = 1
		return
	}

	// Allocate memory in guest for the response
	malloc := mod.ExportedFunction("alloy_malloc")
	if malloc == nil {
		stack[0] = 1
		return
	}

	res, err := malloc.Call(ctx, uint64(len(data)))
	if err != nil || len(res) == 0 {
		stack[0] = 1
		return
	}
	ptr := uint32(res[0])

	if !mem.Write(ptr, data) {
		stack[0] = 1
		return
	}

	if !mem.WriteUint32Le(respPtrPtr, ptr) {
		stack[0] = 1
		return
	}
	if !mem.WriteUint32Le(respSizePtr, uint32(len(data))) {
		stack[0] = 1
		return
	}

	stack[0] = 0
}

func (r *Runtime) InstantiateAlloyHost(ctx context.Context) (wazeroapi.Module, error) {
	if mod := r.r.Module(HostModuleName); mod != nil {
		return mod, nil
	}

	return r.r.NewHostModuleBuilder(HostModuleName).
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, offset, byteCount uint32) {
			buf, ok := mod.Memory().Read(offset, byteCount)
			if ok {
				r.logger.Info("wasm_log", "plugin", mod.Name(), "msg", strings.TrimSpace(string(buf)))
			}
		}).
		Export("log").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, kPtr, kLen, vPtr, vLen uint32) uint32 {
			kBuf, kOk := mod.Memory().Read(kPtr, kLen)
			vBuf, vOk := mod.Memory().Read(vPtr, vLen)
			if !kOk || !vOk { return 1 }
			key := string(kBuf)
			namespace := mod.Name()
			if strings.HasPrefix(key, "shared:") {
				namespace = "shared"
				key = key[7:]
			}
			if err := r.kv.Set(namespace, key, vBuf); err != nil { return 1 }
			return 0
		}).
		Export("kv_set").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, kPtr, kLen, vPtr, vMaxLen uint32) uint32 {
			kBuf, ok := mod.Memory().Read(kPtr, kLen)
			if !ok { return 0 }
			key := string(kBuf)
			namespace := mod.Name()
			if strings.HasPrefix(key, "shared:") {
				namespace = "shared"
				key = key[7:]
			}
			val, err := r.kv.Get(namespace, key)
			if err != nil || val == nil { return 0 }
			if uint32(len(val)) > vMaxLen { return uint32(len(val)) }
			if !mod.Memory().Write(vPtr, val) { return 0 }
			return uint32(len(val))
		}).
		Export("kv_get").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, kPtr, kLen uint32) uint32 {
			kBuf, ok := mod.Memory().Read(kPtr, kLen)
			if !ok { return 1 }
			key := string(kBuf)
			namespace := mod.Name()
			if strings.HasPrefix(key, "shared:") {
				namespace = "shared"
				key = key[7:]
			}
			if err := r.kv.Delete(namespace, key); err != nil { return 1 }
			return 0
		}).
		Export("kv_delete").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, pPtr, pLen, respPtrPtr, respSizePtr uint32) uint32 {
			pBuf, ok := mod.Memory().Read(pPtr, pLen)
			if !ok { return 1 }
			prefix := string(pBuf)
			namespace := mod.Name()
			if strings.HasPrefix(prefix, "shared:") {
				namespace = "shared"
				prefix = prefix[7:]
			}
			keys, err := r.kv.List(namespace, prefix)
			if err != nil { return 1 }
			data, err := json.Marshal(keys)
			if err != nil { return 1 }

			// Allocate in guest
			malloc := mod.ExportedFunction("alloy_malloc")
			if malloc == nil { return 1 }
			res, err := malloc.Call(ctx, uint64(len(data)))
			if err != nil || len(res) == 0 { return 1 }
			ptr := uint32(res[0])

			if !mod.Memory().Write(ptr, data) { return 1 }
			if !mod.Memory().WriteUint32Le(respPtrPtr, ptr) { return 1 }
			if !mod.Memory().WriteUint32Le(respSizePtr, uint32(len(data))) { return 1 }
			return 0
		}).
		Export("kv_list").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, ptr, size, respPtrPtr, respSizePtr uint32) uint32 {
			buf, ok := mod.Memory().Read(ptr, size)
			if !ok { return 1 }
			var msg api.Message
			if err := json.Unmarshal(buf, &msg); err != nil { return 1 }
			resp, err := r.callFn(ctx, msg)
			if err != nil { return 1 }
			data, err := json.Marshal(resp)
			if err != nil { return 1 }

			// Allocate in guest
			malloc := mod.ExportedFunction("alloy_malloc")
			if malloc == nil { return 1 }
			res, err := malloc.Call(ctx, uint64(len(data)))
			if err != nil || len(res) == 0 { return 1 }
			ptrResp := uint32(res[0])

			if !mod.Memory().Write(ptrResp, data) { return 1 }
			if !mod.Memory().WriteUint32Le(respPtrPtr, ptrResp) { return 1 }
			if !mod.Memory().WriteUint32Le(respSizePtr, uint32(len(data))) { return 1 }
			return 0
		}).
		Export("call").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, ptr, size uint32) uint32 {
			buf, ok := mod.Memory().Read(ptr, size)
			if !ok { return 1 }
			var msg api.Message
			if err := json.Unmarshal(buf, &msg); err != nil { return 1 }
			r.routerFn(ctx, msg)
			return 0
		}).
		Export("route_message").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, inPtr, outPtr uint32) {
			r.logger.Info("wasm plugin signaled started (host)", "id", mod.Name())
			// Signal that the plugin has started and initialized its runtime
			if router, ok := ctx.Value("alloy.start_chan").(chan [2]uint32); ok {
				select {
				case router <- [2]uint32{inPtr, outPtr}:
				default:
				}
			}
		}).
		Export("started").
		NewFunctionBuilder().
		WithFunc(r.hostFetch).
		Export("fetch").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context) {
			// Stay alive until context is cancelled (plugin shutdown)
			<-ctx.Done()
		}).
		Export("sleep_forever").
		Instantiate(ctx)
}

func (r *Runtime) hostFetch(ctx context.Context, mod wazeroapi.Module, reqPtr, reqSize, respPtrPtr, respSizePtr uint32) uint32 {
	// 1. Read request from guest
	reqBuf, ok := mod.Memory().Read(reqPtr, reqSize)
	if !ok { return 1 }

	var req struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    []byte            `json:"body"`
	}
	if err := json.Unmarshal(reqBuf, &req); err != nil { return 1 }

	// 2. Perform HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewBuffer(req.Body))
	if err != nil { return 1 }
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		r.logger.Error("host fetch failed", "url", req.URL, "error", err)
		return 1
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	
	// 3. Prepare response for guest
	result := struct {
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers"`
		Body    []byte            `json:"body"`
	}{
		Status:  resp.StatusCode,
		Headers: make(map[string]string),
		Body:    respBody,
	}
	for k, v := range resp.Header {
		result.Headers[k] = v[0]
	}

	resBuf, _ := json.Marshal(result)
	
	// 4. Allocate memory in guest via exported Alloy_malloc (this assumes Alloy_malloc exists)
	// We need to call back into the guest to allocate space for the result.
	malloc := mod.ExportedFunction("alloy_malloc")
	if malloc == nil { return 1 }

	results, err := malloc.Call(ctx, uint64(len(resBuf)))
	if err != nil { return 1 }
	outPtr := uint32(results[0])

	// 5. Write results into allocated guest memory
	if !mod.Memory().Write(outPtr, resBuf) { return 1 }

	// 6. Write pointer and size back to out parameters
	if !mod.Memory().WriteUint32Le(respPtrPtr, outPtr) { return 1 }
	if !mod.Memory().WriteUint32Le(respSizePtr, uint32(len(resBuf))) { return 1 }

	return 0
}
