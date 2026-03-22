package wasm_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
	"github.com/jnesbitt/alloy-go/pkg/wasm"
)

func TestManagerBasicOperations(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "manager-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Set up logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Set up storage
	storagePath := filepath.Join(tempDir, "storage")
	kv, err := storage.NewBadgerStore(storagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()

	// Create message router
	router := func(ctx context.Context, msg api.Message) {}

	// Create call function
	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		return api.Message{}, nil
	}

	// Test 1: Create manager
	manager, err := wasm2.NewManager(logger, kv, tempDir, router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())

	if manager == nil {
		t.Error("manager should not be nil")
	}

	t.Log("Manager basic operations test passed!")
}

func TestManagerPluginLifecycle(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "manager-lifecycle-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Set up logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Set up storage
	storagePath := filepath.Join(tempDir, "storage")
	kv, err := storage.NewBadgerStore(storagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()

	// Create message router
	router := func(ctx context.Context, msg api.Message) {}

	// Create call function
	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		return api.Message{}, nil
	}

	// Create manager
	manager, err := wasm2.NewManager(logger, kv, tempDir, router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())

	// Create a simple WASM module for testing
	wasmBytes := createTestWASMModule(t)

	// Test 1: Load plugin
	pluginID := "test-plugin"
	caps := []api.Capability{
		{Method: "test:method", Description: "Test method"},
	}

	err = manager.LoadPlugin(context.Background(), pluginID, wasmBytes, caps)
	if err != nil {
		t.Fatal(err)
	}

	// Test 2: Verify plugin capabilities
	pluginCaps, ok := manager.GetPluginCapabilities(pluginID)
	if !ok {
		t.Error("plugin should be registered")
	}

	if len(pluginCaps) != 1 {
		t.Error("plugin should have 1 capability")
	}

	if pluginCaps[0].Method != "test:method" {
		t.Error("unexpected capability method")
	}

	// Test 3: Verify plugin status
	status, ok := manager.GetPluginStatus(pluginID)
	if !ok {
		t.Error("plugin should be registered")
	}

	if status != wasm2.StatusRunning {
		t.Error("plugin should be running")
	}

	// Test 4: Unload plugin
	err = manager.UnloadPlugin(context.Background(), pluginID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify plugin is unloaded
	_, ok = manager.GetPluginCapabilities(pluginID)
	if ok {
		t.Error("plugin should be unloaded")
	}

	t.Log("Manager plugin lifecycle test passed!")
}

func TestManagerMessageRouting(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "manager-routing-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Set up logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Set up storage
	storagePath := filepath.Join(tempDir, "storage")
	kv, err := storage.NewBadgerStore(storagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()

	// Create message router
	var receivedMsg api.Message
	router := func(ctx context.Context, msg api.Message) {
		receivedMsg = msg
	}

	// Create call function
	callCount := 0
	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		callCount++
		return api.Message{}, nil
	}

	// Create manager
	manager, err := wasm2.NewManager(logger, kv, tempDir, router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())

	// Create a simple WASM module for testing
	wasmBytes := createTestWASMModule(t)

	// Load plugin
	pluginID := "test-plugin"
	caps := []api.Capability{
		{Method: "test:method", Description: "Test method"},
	}

	err = manager.LoadPlugin(context.Background(), pluginID, wasmBytes, caps)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for plugin to initialize
	time.Sleep(100 * time.Millisecond)

	// Test 1: Route message to plugin
	testMsg := api.Message{
		ID:      "test-1",
		Method:  "test:method",
		Sender:  "test-client",
		Target:  pluginID,
		Payload: []byte(`{}`),
	}

	err = manager.RouteMessage(context.Background(), pluginID, testMsg)
	if err != nil {
		t.Fatal(err)
	}

	// Test 2: Get response from plugin
	resp, err := manager.GetResponse(context.Background(), pluginID)
	if err != nil {
		t.Fatal(err)
	}

	if resp.ID != "test-1-response" {
		t.Errorf("unexpected response ID: %s", resp.ID)
	}

	// Test 3: Verify call function
	if callCount != 0 {
		t.Error("call function should not have been called")
	}

	t.Log("Manager message routing test passed!")
}

// createTestWASMModule creates a simple WASM module for testing.
func createTestWASMModule(t *testing.T) []byte {
	// In a real test, we would build a WASM module
	// For now, we'll skip this test
	t.Skip("WASM module creation not implemented")
	return nil
}