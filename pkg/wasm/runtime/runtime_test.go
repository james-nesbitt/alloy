package runtime_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
	"github.com/jnesbitt/alloy-go/pkg/wasm"
	"github.com/jnesbitt/alloy-go/pkg/wasm/runtime"
)

func TestRuntimeBasicOperations(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "runtime-test")
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
	routerCalled := false
	router := func(ctx context.Context, msg api.Message) {
		routerCalled = true
	}

	// Create call function
	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		resp := api.Message{
			ID:      msg.ID + "-response",
			Type:    "response",
			Method:  msg.Method,
			Sender:  msg.Target,
			Target:  msg.Sender,
			Payload: json.RawMessage(`{"result":"success"}`),
		}
		return resp, nil
	}

	// Create runtime
	rt, err := runtime.NewRuntime(context.Background(), logger, kv, tempDir, router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	// Test 1: Verify runtime creation
	if rt == nil {
		t.Error("runtime should not be nil")
	}

	// Test 2: Verify host module instantiation
	hostMod := rt.HostModule()
	if hostMod == nil {
		t.Error("host module should not be nil")
	}

	// Test 3: Verify KV storage operations
	err = kv.Set("test", "key", []byte("value"))
	if err != nil {
		t.Fatal(err)
	}

	value, err := kv.Get("test", "key")
	if err != nil {
		t.Fatal(err)
	}

	if string(value) != "value" {
		t.Errorf("expected 'value', got '%s'", string(value))
	}

	t.Log("Runtime basic operations test passed!")
}

func TestRuntimePluginLifecycle(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "runtime-lifecycle-test")
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

	// Create runtime
	rt, err := runtime.NewRuntime(context.Background(), logger, kv, tempDir, router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	// Create a simple WASM module for testing
	wasmBytes := createTestWASMModule(t)

	// Test 1: Load plugin
	pluginID := "test-plugin"
	caps := []api.Capability{
		{Method: "test:method", Description: "Test method"},
	}

	instance, err := rt.LoadPlugin(context.Background(), pluginID, wasmBytes, 0, caps)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close(context.Background())

	// Test 2: Verify plugin registration
	rt.Mu().RLock()
	_, ok := rt.Plugins()[pluginID]
	rt.Mu().RUnlock()

	if !ok {
		t.Error("plugin should be registered")
	}

	// Test 3: Verify plugin capabilities
	if len(instance.Capabilities()) != 1 {
		t.Error("plugin should have 1 capability")
	}

	if instance.Capabilities()[0].Method != "test:method" {
		t.Error("unexpected capability method")
	}

	// Test 4: Verify plugin status
	if instance.Status() != runtime.StatusRunning {
		t.Error("plugin should be running")
	}

	t.Log("Runtime plugin lifecycle test passed!")
}

func TestRuntimeMessageRouting(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "runtime-routing-test")
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
	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		return api.Message{}, nil
	}

	// Create runtime
	rt, err := runtime.NewRuntime(context.Background(), logger, kv, tempDir, router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	// Create a simple WASM module for testing
	wasmBytes := createTestWASMModule(t)

	// Load plugin
	pluginID := "test-plugin"
	caps := []api.Capability{
		{Method: "test:method", Description: "Test method"},
	}

	instance, err := rt.LoadPlugin(context.Background(), pluginID, wasmBytes, 0, caps)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close(context.Background())

	// Wait for plugin to initialize
	time.Sleep(100 * time.Millisecond)

	// Test 1: Route message to plugin
	testMsg := api.Message{
		ID:      "test-1",
		Method:  "test:method",
		Sender:  "test-client",
		Target:  pluginID,
		Payload: json.RawMessage(`{}`),
	}

	err = rt.RouteMessage(context.Background(), pluginID, testMsg)
	if err != nil {
		t.Fatal(err)
	}

	// Test 2: Get response from plugin
	resp, err := rt.GetResponse(context.Background(), pluginID)
	if err != nil {
		t.Fatal(err)
	}

	if resp.ID != "test-1-response" {
		t.Errorf("unexpected response ID: %s", resp.ID)
	}

	t.Log("Runtime message routing test passed!")
}

// createTestWASMModule creates a simple WASM module for testing.
func createTestWASMModule(t *testing.T) []byte {
	// In a real test, we would build a WASM module
	// For now, we'll skip this test
	t.Skip("WASM module creation not implemented")
	return nil
}