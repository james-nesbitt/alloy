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

// Runtime manages the wazero WASM environment.
type Runtime struct {
	logger *slog.Logger
	r      wazero.Runtime
}

// NewRuntime creates a new Alloy WASM runtime.
func NewRuntime(ctx context.Context, logger *slog.Logger) (*Runtime, error) {
	// Configuration for resource constraints
	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCoreFeatures(api.CoreFeaturesV2))

	// Instantiate WASI
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	return &Runtime{
		logger: logger,
		r:      r,
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
	return r.r.NewHostModuleBuilder("alloy").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, offset, byteCount uint32) {
			buf, ok := mod.Memory().Read(offset, byteCount)
			if !ok {
				fmt.Printf("failed to read memory for log\n")
				return
			}
			r.logger.Info("wasm_log", "msg", string(buf))
		}).
		Export("log").
		Instantiate(ctx)
}
