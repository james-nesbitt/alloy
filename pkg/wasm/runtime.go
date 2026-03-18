package wasm

import (
	"context"
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
	logger  *slog.Logger
	r       wazero.Runtime
	kv      storage.StateStore
	dataDir string
}

// NewRuntime creates a new Alloy WASM runtime.
func NewRuntime(ctx context.Context, logger *slog.Logger, kv storage.StateStore, dataDir string) (*Runtime, error) {
	// Configuration for resource constraints.
	// Allow 500MB total host memory.
	nc := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(wazeroapi.CoreFeaturesV2).
		WithMemoryLimitPages(500 * 1024 * 1024 / WasmPageSize)

	rt := wazero.NewRuntimeWithConfig(ctx, nc)

	// Ensure dataDir exists
	if dataDir != "" {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create runtime data directory: %w", err)
		}
	}

	// Instantiate WASI properly
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	return &Runtime{
		logger:  logger,
		r:       rt,
		kv:      kv,
		dataDir: dataDir,
	}, nil
}

func (r *Runtime) LoadPlugin(ctx context.Context, id string, wasmBytes []byte, memoryLimitMB uint64, fuelLimit uint64) (api.Plugin, error) {
	config := wazero.NewModuleConfig().
		WithName(id).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithSysWalltime().
		WithSysNanotime()

	// Map storage for the plugin
	if r.dataDir != "" {
		pluginDir := filepath.Join(r.dataDir, id)
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create plugin storage dir: %w", err)
		}
		config = config.WithFS(os.DirFS(pluginDir))
		r.logger.Debug("mapped plugin storage", "id", id, "path", pluginDir)
	}

	if memoryLimitMB > 0 {
		r.logger.Debug("resource limits requested but per-module memory limiting is constrained in this version", "id", id, "limit_mb", memoryLimitMB)
	}

	// Instantiate module but don't call _start automatically.
	// For Go wasip1, _start is actually main.main.
	compiled, err := r.r.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to compile module %s: %w", id, err)
	}

	mod, err := r.r.InstantiateModule(ctx, compiled, config)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate module %s: %w", id, err)
	}

	// Run _start asynchronously.
	if start := mod.ExportedFunction("_start"); start != nil {
		go func() {
			// This goroutine keeps the Go plugin's "main" alive.
			_, err := start.Call(context.Background())
			if err != nil {
				r.logger.Debug("wasm plugin _start returned", "id", id, "error", err)
			}
		}()
		// Allow some time for runtime stabilization
		time.Sleep(100 * time.Millisecond)
	}

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
	// Idempotent host module instantiation
	if mod := r.r.Module(HostModuleName); mod != nil {
		return mod, nil
	}

	return r.r.NewHostModuleBuilder(HostModuleName).
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, offset, byteCount uint32) {
			buf, modMemoryReadOk := mod.Memory().Read(offset, byteCount)
			if !modMemoryReadOk {
				return
			}
			r.logger.Info("wasm_log", "plugin", mod.Name(), "msg", string(buf))
		}).
		Export("log").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, kPtr, kLen, vPtr, vLen uint32) uint32 {
			kBuf, kOk := mod.Memory().Read(kPtr, kLen)
			vBuf, vOk := mod.Memory().Read(vPtr, vLen)
			if !kOk || !vOk {
				return 1 // Error
			}
			if err := r.kv.Set(mod.Name(), string(kBuf), vBuf); err != nil {
				return 1
			}
			return 0 // Success
		}).
		Export("kv_set").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, kPtr, kLen, vPtr, vMaxLen uint32) uint32 {
			kBuf, ok := mod.Memory().Read(kPtr, kLen)
			if !ok {
				return 0
			}
			val, err := r.kv.Get(mod.Name(), string(kBuf))
			if err != nil || val == nil {
				return 0
			}
			if uint32(len(val)) > vMaxLen {
				return uint32(len(val)) // Return required size
			}
			if !mod.Memory().Write(vPtr, val) {
				return 0
			}
			return uint32(len(val))
		}).
		Export("kv_get").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod wazeroapi.Module, kPtr, kLen uint32) uint32 {
			kBuf, ok := mod.Memory().Read(kPtr, kLen)
			if !ok {
				return 1
			}
			if err := r.kv.Delete(mod.Name(), string(kBuf)); err != nil {
				return 1
			}
			return 0
		}).
		Export("kv_delete").
		Instantiate(ctx)
}
