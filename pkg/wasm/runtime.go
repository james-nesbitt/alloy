package wasm

import (
	"context"
	"fmt"
	"log/slog"

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
	r := wazero.NewRuntime(ctx)
	
	// Instantiate WASI
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	return &Runtime{
		logger: logger,
		r:      r,
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
		ExportFunction("log", func(ctx context.Context, mod api.Module, offset, byteCount uint32) {
			buf, ok := mod.Memory().Read(offset, byteCount)
			if !ok {
				fmt.Printf("failed to read memory for log\n")
				return
			}
			r.logger.Info("wasm_log", "msg", string(buf))
		}).
		Instantiate(ctx)
}
