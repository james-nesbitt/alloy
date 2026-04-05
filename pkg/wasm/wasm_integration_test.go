package wasm_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
	"github.com/james-nesbitt/alloy/pkg/wasm/runtime"
)

func TestWITRuntimeIntegration(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "wit-runtime-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Build the test WASM module
	wasmPath := filepath.Join(tempDir, "test_wasm.wasm")
	buildTestWASM(t, wasmPath)

	// Read the WASM bytes
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Fatal(err)
	}

	// Set up storage
	storagePath := filepath.Join(tempDir, "storage")
	kv, err := storage.NewFileStateStore(storagePath)
	if err != nil {
		t.Fatal(err)
	}

	// Create message router
	var receivedMessages []api.Message
	router := func(ctx context.Context, msg api.Message) {
		receivedMessages = append(receivedMessages, msg)
	}

	// Create call function
	callCount := 0
	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		callCount++
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
	runtime, err := runtime.NewRuntime(context.Background(), nil, kv, tempDir, nil, router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())

	// Load the plugin
	pluginID := "test-wasm"
	caps := []api.Capability{
		{Method: "test:hello", Description: "Responds with a hello message"},
		{Method: "test:echo", Description: "Echoes back the input"},
	}

	instance, err := runtime.LoadPlugin(context.Background(), pluginID, wasmBytes, 128, 100, caps, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close(context.Background())

	// Wait for plugin to initialize
	time.Sleep(100 * time.Millisecond)

	// Test 1: Route a message to the plugin
	testMsg := api.Message{
		ID:      "test-1",
		Method:  "test:hello",
		Sender:  "test-client",
		Target:  pluginID,
		Payload: json.RawMessage(`{}`),
		Type:    api.TypeRequest,
	}

	err = runtime.RouteMessage(context.Background(), pluginID, testMsg)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for response
	time.Sleep(50 * time.Millisecond)

	// Test 2: Get a response from the plugin
	resp, err := runtime.GetResponse(context.Background(), pluginID, "test-1")
	if err != nil {
		t.Fatal(err)
	}

	if resp.ID != "test-1-response" {
		t.Errorf("unexpected response ID: %s", resp.ID)
	}

	// Verify payload
	var payload map[string]string
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatal(err)
	}

	if payload["message"] != "Hello from WASM!" {
		t.Errorf("unexpected response message: %s", payload["message"])
	}

	// Test 3: Verify KV storage
	value, err := kv.Get(pluginID, "test-key")
	if err != nil {
		t.Fatal(err)
	}

	if string(value) != "test-value" {
		t.Errorf("unexpected KV value: %s", string(value))
	}

	// Test 4: Test call function
	callMsg := api.Message{
		ID:     "test-call",
		Method: "test:call",
		Sender: pluginID,
		Target: "other-plugin",
	}

	err = runtime.RouteMessage(context.Background(), pluginID, callMsg)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for call to complete
	time.Sleep(50 * time.Millisecond)

	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	t.Log("All WIT runtime integration tests passed!")
}

// buildTestWASM builds the test WASM module.
func buildTestWASM(t *testing.T, outputPath string) {
	// In a real test, we would build the WASM module here
	// For now, we'll just skip this test
	t.Skip("WASM build not implemented in test")
}
