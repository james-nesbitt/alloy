package tests

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
	"github.com/jnesbitt/alloy-go/pkg/wasm"
)

func TestStandaloneWasmLoad(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	kv := storage.NewMemoryStateStore()
	dataDir, _ := os.MkdirTemp("", "alloy-wasm-standalone-*")
	defer os.RemoveAll(dataDir)

	// Initialize the runtime
	rt, err := wasm.NewRuntime(ctx, logger, kv, dataDir)
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}
	defer rt.Close(ctx)

	// Instantiate the host module
	if _, err := rt.InstantiateAlloyHost(ctx); err != nil {
		t.Fatalf("failed to instantiate host module: %v", err)
	}

	// Load the mock plugin binary
	wasmPath := "../plugins/wasm/mock/mock.wasm"
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Fatalf("failed to read wasm file: %v", err)
	}

	t.Logf("Loading mock plugin from %s (%d bytes)", wasmPath, len(wasmBytes))

	// Instantiate the plugin
	plugin, err := rt.LoadPlugin(ctx, "mock-plugin", wasmBytes, 64, 0)
	if err != nil {
		t.Fatalf("failed to load plugin: %v", err)
	}

	if plugin == nil {
		t.Fatal("plugin is nil after load")
	}

	t.Logf("Successfully loaded plugin: %s", plugin.ID())

	// Test a message
	resp, err := plugin.HandleMessage(ctx, api.Message{
		ID:        "ping-1",
		Type:      api.TypeRequest,
		Sender:    "test-runner",
		Target:    "mock-plugin",
		Method:    "ping",
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		t.Errorf("failed to handle message: %v", err)
	} else {
		t.Logf("Successfully handled message: %s - response: %s", resp.ID, string(resp.Payload))
	}

	// Shutdown the plugin
	if err := plugin.Shutdown(ctx); err != nil {
		t.Errorf("failed to shutdown plugin: %v", err)
	}
}
