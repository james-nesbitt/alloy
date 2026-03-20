package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
}

// NewRuntime creates a new Alloy WASM runtime.
func NewRuntime(ctx context.Context, logger *slog.Logger, kv storage.StateStore, dataDir string, router func(context.Context, api.Message)) (*Runtime, error) {
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
	}, nil
}

func (r *Runtime) LoadPlugin(ctx context.Context, id string, wasmBytes []byte, memoryLimitMB uint64, fuelLimit uint64) (api.Plugin, error) {
	config := wazero.NewModuleConfig().
		WithName(id).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
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

	r.logger.Info("compiling wasm module", "id", id, "size", len(wasmBytes))
	compiled, err := r.r.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to compile module %s: %w", id, err)
	}
	r.logger.Info("compiled wasm module", "id", id)

	// For Go libraries (-buildmode=c-shared), we can instantiate synchronously as it won't block
	// For Go commands, we must instantiate in a goroutine because they block in main
	r.logger.Info("instantiating wasm module", "id", id)

	var mod wazeroapi.Module
	if initFunc := compiled.ExportedFunctions()["_initialize"]; initFunc != nil {
		// Reactor/Library mode
		m, err := r.r.InstantiateModule(ctx, compiled, config)
		if err != nil {
			return nil, fmt.Errorf("failed to instantiate module %s: %w", id, err)
		}
		mod = m
		r.logger.Info("initializing wasm runtime", "id", id)
		if _, err := mod.ExportedFunction("_initialize").Call(ctx); err != nil {
			return nil, fmt.Errorf("failed to initialize module %s: %w", id, err)
		}
	} else {
		// Command mode (blocks in _start)
		errChan := make(chan error, 1)
		modChan := make(chan wazeroapi.Module, 1)
		go func() {
			m, err := r.r.InstantiateModule(ctx, compiled, config)
			if err != nil {
				errChan <- err
				return
			}
			modChan <- m
		}()

		select {
		case m := <-modChan:
			mod = m
		case err := <-errChan:
			return nil, fmt.Errorf("failed to instantiate module %s: %w", id, err)
		case <-time.After(10 * time.Second):
			mod = r.r.Module(id)
			if mod == nil {
				return nil, fmt.Errorf("timeout waiting for module %s to instantiate", id)
			}
		}
	}

	r.logger.Info("instantiated wasm module", "id", id)

	return &Instance{
		id:          id,
		mod:         mod,
		logger:      r.logger,
		defaultFuel: fuelLimit,
	}, nil
}

// Close shuts down the runtime.
func (r *Runtime) Close(ctx context.Context) error {
	return r.r.Close(ctx)
}

// InstantiateAlloyHost defines the host-side functions available to plugins.
func (r *Runtime) InstantiateAlloyHost(ctx context.Context) (wazeroapi.Module, error) {
	if mod := r.r.Module(HostModuleName); mod != nil {
		return mod, nil
	}

	return r.r.NewHostModuleBuilder(HostModuleName).
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, offset, byteCount uint32) {
			buf, ok := mod.Memory().Read(offset, byteCount)
			if ok {
				fmt.Fprintf(os.Stderr, "[WASM LOG] %s: %s\n", mod.Name(), string(buf))
				r.logger.Info("wasm_log", "plugin", mod.Name(), "msg", string(buf))
			}
		}).
		Export("log").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, kPtr, kLen, vPtr, vLen uint32) uint32 {
			kBuf, kOk := mod.Memory().Read(kPtr, kLen)
			vBuf, vOk := mod.Memory().Read(vPtr, vLen)
			if !kOk || !vOk { return 1 }
			if err := r.kv.Set(mod.Name(), string(kBuf), vBuf); err != nil { return 1 }
			return 0
		}).
		Export("kv_set").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, kPtr, kLen, vPtr, vMaxLen uint32) uint32 {
			kBuf, ok := mod.Memory().Read(kPtr, kLen)
			if !ok { return 0 }
			val, err := r.kv.Get(mod.Name(), string(kBuf))
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
			if err := r.kv.Delete(mod.Name(), string(kBuf)); err != nil { return 1 }
			return 0
		}).
		Export("kv_delete").
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
		WithFunc(func(ctx context.Context) {
			// Stay alive until context is cancelled (plugin shutdown)
			<-ctx.Done()
		}).
		Export("sleep_forever").
		Instantiate(ctx)
}
