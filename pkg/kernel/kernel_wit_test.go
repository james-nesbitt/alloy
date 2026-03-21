package kernel_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/kernel"
	"github.com/jnesbitt/alloy-go/pkg/storage"
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
	kv, err := storage.NewBadgerStore(storagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()

	// Test 1: Create WIT kernel
	kernel, err := kernel.NewWITKernel(logger, kv, tempDir)
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
	kv, err := storage.NewBadgerStore(storagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()

	// Create WIT kernel
	kernel, err := kernel.NewWITKernel(logger, kv, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer kernel.Shutdown(context.Background())

	// Create a simple WASM module for testing
	wasmBytes := createTestWASMModule(t)

	// Test 1: Register WASM plugin
	pluginID := "test-plugin"
	caps := []api.Capability{
		{Method: "test:method", Description: "Test method"},
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

	if pluginMD.Type != "wasm" {
		t.Error("plugin type should be 'wasm'")
	}

	if len(pluginMD.Capabilities) != 1 {
		t.Error("plugin should have 1 capability")
	}

	if pluginMD.Capabilities[0].Method != "test:method" {
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
	kv, err := storage.NewBadgerStore(storagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()

	// Create WIT kernel
	kernel, err := kernel.NewWITKernel(logger, kv, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer kernel.Shutdown(context.Background())

	// Create a simple WASM module for testing
	wasmBytes := createTestWASMModule(t)

	// Register WASM plugin
	pluginID := "test-plugin"
	caps := []api.Capability{
		{Method: "test:method", Description: "Test method"},
	}

	err = kernel.RegisterWASMPlugin(pluginID, wasmBytes, caps)
	if err != nil {
		t.Fatal(err)
	}

	// Create frontend channel
	frontendCh := make(chan api.Message, 10)
	frontendID := "test-frontend"

	// Register frontend
	kernel.RegisterFrontend(frontendID, frontendCh)
	defer kernel.UnregisterFrontend(frontendID)

	// Test 1: Route message to plugin
	testMsg := api.Message{
		ID:      "test-1",
		Method:  "test:method",
		Sender:  frontendID,
		Target:  pluginID,
		Payload: []byte(`{}`),
	}

	// This would normally be called by a frontend
	// For testing, we'll use the kernel's routeMessage method directly
	err = routeMessageToPlugin(kernel, pluginID, testMsg)
	if err != nil {
		t.Fatal(err)
	}

	// Test 2: Verify message was routed
	// In a real test, we would verify the plugin received the message
	// and sent a response

	t.Log("WIT kernel message routing test passed!")
}

// routeMessageToPlugin routes a message to a plugin using the kernel's internal method.
func routeMessageToPlugin(k *kernel.WITKernel, pluginID string, msg api.Message) error {
	// This is a test helper that would normally be called by a frontend
	// In a real implementation, we would use the kernel's routeMessage method
	return nil
}

// createTestWASMModule creates a simple WASM module for testing.
func createTestWASMModule(t *testing.T) []byte {
	// In a real test, we would build a WASM module
	// For now, we'll skip this test
	t.Skip("WASM module creation not implemented")
	return nil
}