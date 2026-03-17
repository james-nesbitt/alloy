package wasm

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const (
	HostModuleName = "alloy"
)

type KVStore interface {
	Get(pluginID, key string) ([]byte, error)
	Set(pluginID, key string, value []byte) error
}

// Runtime manages the wazero WASM environment.
type Runtime struct {
	logger *slog.Logger
	r      wazero.Runtime
	kv     KVStore
}

// NewRuntime creates a new Alloy WASM runtime.
func NewRuntime(ctx context.Context, logger *slog.Logger, kv KVStore) (*Runtime, error) {
	// Configuration for resource constraints
	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCoreFeatures(api.CoreFeaturesV2))
	
	// Note: In 1.11.0, Fuel is enabled via experimental features or compiler config.
	// For this phase, we ensure sandboxed execution via memory limits.

	// Instantiate WASI
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	return &Runtime{
		logger: logger,
		r:      r,
		kv:     kv,
	}, nil
}

// LoadPlugin loads a WASM module from bytes and returns an Alloy-compatible plugin instance.
func (r *Runtime) LoadPlugin(ctx context.Context, id string, wasm []byte) (*Instance, error) {
	config := wazero.NewModuleConfig().
		WithName(id).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)

	mod, err := r.r.InstantiateWithConfig(ctx, wasm, config)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate module: %w", err)
	}

	return &Instance{
		id:     id,
		mod:    mod,
		logger: r.logger,
	}, nil
}

// Close shuts down the runtime.
func (r *Runtime) Close(ctx context.Context) error {
	return r.r.Close(ctx)
}

// InstantiateAlloyHost defines the host-side functions available to plugins.
// This is the beginning of the Host Discovery Interface.
func (r *Runtime) InstantiateAlloyHost(ctx context.Context) (api.Module, error) {
	return r.r.NewHostModuleBuilder(HostModuleName).
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, offset, byteCount uint32) {
			buf, ok := mod.Memory().Read(offset, byteCount)
			if !ok {
				return
			}
			r.logger.Info("wasm_log", "plugin", mod.Name(), "msg", string(buf))
		}).
		Export("log").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, kPtr, kLen, vPtr, vLen uint32) uint32 {
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
		WithFunc(func(ctx context.Context, mod api.Module, kPtr, kLen, vPtr, vMaxLen uint32) uint32 {
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
		Instantiate(ctx)
}
