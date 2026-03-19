package wasm

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jnesbitt/alloy-go/pkg/storage"
)

func TestWasmGoOrchestration(t *testing.T) {
	// This test verifies that the Go structures are correctly initialized.
	// Real WASM execution tests will be added once we have a stable build process for plugins.
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	kv := storage.NewMemoryStateStore()
	dataDir, _ := os.MkdirTemp("", "alloy-wasm-test-*")
	defer os.RemoveAll(dataDir)

	r, err := NewRuntime(ctx, logger, kv, dataDir, nil)
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}
	defer r.Close(ctx)

	if r == nil {
		t.Fatal("runtime is nil")
	}

	hostMod, err := r.InstantiateAlloyHost(ctx)
	if err != nil {
		t.Fatalf("failed to instantiate host module: %v", err)
	}
	if hostMod == nil {
		t.Fatal("host module is nil")
	}
}
