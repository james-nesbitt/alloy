package kernel_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/kernel"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

func TestKernelBasicOperations(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "kernel-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Set up logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Set up storage
	storagePath := filepath.Join(tempDir, "storage")
	kv, err := storage.NewFileStateStore(storagePath)
	if err != nil {
		t.Fatal(err)
	}

	// Test 1: Create kernel
	k, err := kernel.New(logger, kv, tempDir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer k.Shutdown(context.Background())

	if k == nil {
		t.Error("kernel should not be nil")
	}

	t.Log("Kernel basic operations test passed!")
}

func TestKernelPluginRegistration(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "kernel-registration-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Set up logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Set up storage
	storagePath := filepath.Join(tempDir, "storage")
	kv, err := storage.NewFileStateStore(storagePath)
	if err != nil {
		t.Fatal(err)
	}

	// Create kernel
	k, err := kernel.New(logger, kv, tempDir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer k.Shutdown(context.Background())

	// Create a simple WASM module for testing
	wasmBytes := createTestWASMModule(t)

	// Test 1: Register WASM plugin
	pluginID := "test-plugin"
	caps := []api.Capability{
		{Method: "status", Description: "Test method"},
	}

	err = k.RegisterWASMPlugin(pluginID, wasmBytes, caps)
	if err != nil {
		t.Fatal(err)
	}

	// Test 2: Verify plugin metadata
	metadata := k.GetPluginMetadata()
	pluginMD, ok := metadata[pluginID]
	if !ok {
		t.Error("plugin should be registered")
	}

	if len(pluginMD.Capabilities) != 1 {
		t.Error("plugin should have 1 capability")
	}

	if pluginMD.Capabilities[0].Method != "status" {
		t.Error("unexpected capability method")
	}

	t.Log("Kernel plugin registration test passed!")
}

func TestKernelMessageRouting(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "kernel-routing-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Set up logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Set up storage
	storagePath := filepath.Join(tempDir, "storage")
	kv, err := storage.NewFileStateStore(storagePath)
	if err != nil {
		t.Fatal(err)
	}

	// Create kernel
	k, err := kernel.New(logger, kv, tempDir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer k.Shutdown(context.Background())

	// Read the health plugin for testing
	cwd, _ := os.Getwd()
	// Navigate up from pkg/kernel to project root
	projectRoot := filepath.Dir(filepath.Dir(cwd))
	healthWasmPath := filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/health.wasm")
	healthWasm, err := os.ReadFile(healthWasmPath)
	if err != nil {
		t.Errorf("failed to read health wasm at %s: %v", healthWasmPath, err)
		return
	}

	// Register WASM plugin
	pluginID := "health"
	caps := []api.Capability{
		{Method: "status", Description: "Get health status"},
	}

	err = k.RegisterWASMPlugin(pluginID, healthWasm, caps)
	if err != nil {
		t.Fatal(err)
	}

	// Create frontend channel
	frontendCh := make(chan api.Message, 10)
	frontendID := "test-user"

	// Register frontend
	k.RegisterFrontend(frontendID, frontendCh)
	defer k.UnregisterFrontend(frontendID)

	// Test 1: Route message to plugin
	testMsg := api.Message{
		ID:      "test-1",
		Type:    api.TypeRequest,
		Method:  "status",
		Sender:  frontendID,
		Target:  pluginID,
		Payload: []byte(`{}`),
	}

	// Route the message through kernel
	k.RouteMessage(context.Background(), testMsg)

	// Test 2: Verify message was routed and we got a response in the frontend
	select {
	case resp := <-frontendCh:
		if resp.ID != "test-1-resp" {
			t.Errorf("unexpected response ID: %s", resp.ID)
		}
		if resp.Sender != "health" {
			t.Errorf("unexpected response sender: %s", resp.Sender)
		}
		var payload map[string]any
		if err := json.Unmarshal(resp.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		t.Logf("Response payload: %v", payload)
		// Basic check that we got a valid response
		if _, ok := payload["uptime"]; !ok && payload["status"] == nil {
			t.Errorf("unexpected response content: %v", payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for response from plugin")
	}

	t.Log("Kernel message routing test passed!")
}

func createTestWASMModule(t *testing.T) []byte {
	// Re-use built-in plugins for kernel testing
	cwd, _ := os.Getwd()
	projectRoot := filepath.Dir(filepath.Dir(cwd))
	healthWasm, err := os.ReadFile(filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/health.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	return healthWasm
}
