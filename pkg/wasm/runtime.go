package wasm

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
	"github.com/tetratelabs/wazero"
	wazeroapi "github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const (
	HostModuleName = "alloy"
)

// Runtime manages the wazero WASM environment.
type Runtime struct {
	logger *slog.Logger
	r      wazero.Runtime
	kv     storage.StateStore
}

// NewRuntime creates a new Alloy WASM runtime.
func NewRuntime(ctx context.Context, logger *slog.Logger, kv storage.StateStore) (*Runtime, error) {
	// Configuration for resource constraints.
	// We use the interpreter for now because the compiler is too slow for integration tests in some environments.
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(wazeroapi.CoreFeaturesV2))

	// Instantiate WASI
	if _, err := wasi_snapshot_preview1.NewBuilder(rt).Instantiate(ctx); err != nil {
		return nil, fmt.Errorf("failed to instantiate WASI: %w", err)
	}

	return &Runtime{
		logger: logger,
		r:      rt,
		kv:     kv,
	}, nil
}

func (r *Runtime) LoadPlugin(ctx context.Context, id string, wasmBytes []byte, memoryLimitMB uint64, fuelLimit uint64) (api.Plugin, error) {
	config := wazero.NewModuleConfig().
		WithName(id).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithSysWalltime().
		WithSysNanotime()

	// Use InstantiateWithConfig which handles _start correctly for Go WASIP1
	mod, err := r.r.InstantiateWithConfig(ctx, wasmBytes, config)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate module %s: %w", id, err)
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
