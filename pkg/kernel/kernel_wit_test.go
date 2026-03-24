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

func TestWITKernelBasicOperations(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "wit-kernel-test")
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

	// Test 1: Create WIT kernel
	kernel, err := kernel.NewWITKernel(logger, kv, tempDir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer kernel.Shutdown(context.Background())

	if kernel == nil {
		t.Error("kernel should not be nil")
	}

	t.Log("WIT kernel basic operations test passed!")
}

func TestWITKernelPluginRegistration(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "wit-kernel-registration-test")
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

	// Create WIT kernel
	kernel, err := kernel.NewWITKernel(logger, kv, tempDir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer kernel.Shutdown(context.Background())

	// Create a simple WASM module for testing
	wasmBytes := createTestWASMModule(t)

	// Test 1: Register WASM plugin
	pluginID := "test-plugin"
	caps := []api.Capability{
		{Method: "status", Description: "Test method"},
	}

	err = kernel.RegisterWASMPlugin(pluginID, wasmBytes, caps)
	if err != nil {
		t.Fatal(err)
	}

	// Test 2: Verify plugin metadata
	metadata := kernel.GetPluginMetadata()
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

	t.Log("WIT kernel plugin registration test passed!")
}

func TestWITKernelMessageRouting(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "wit-kernel-routing-test")
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

	// Create WIT kernel
	k, err := kernel.NewWITKernel(logger, kv, tempDir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer k.Shutdown(context.Background())

	// Read the health plugin for testing
	cwd, _ := os.Getwd()
	projectRoot := filepath.Dir(filepath.Dir(cwd))
	healthWasm, err := os.ReadFile(filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/health.wasm"))
	if err != nil {
		t.Fatal(err)
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

	// Route the message through kernel (this should trigger IAM check if present, 
	// which is allowed for health:status by default as per iam_interceptor logic)
	go k.RouteMessage(context.Background(), testMsg)

	// Test 2: Verify message was routed and we got a response in the frontend
	select {
	case resp := <-frontendCh:
		if resp.ID != "test-1-resp" {
			t.Errorf("unexpected response ID: %s", resp.ID)
		}
		if resp.Sender != "health" {
			t.Errorf("unexpected response sender: %s", resp.Sender)
		}
		var payload map[string]string
		if err := json.Unmarshal(resp.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["status"] != "healthy" {
			t.Errorf("unexpected status: %s", payload["status"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for response from plugin")
	}

	t.Log("WIT kernel message routing test passed!")
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
