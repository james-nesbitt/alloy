package tests

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
	"github.com/jnesbitt/alloy-go/pkg/wasm2"
)

func TestWITIntegration(t *testing.T) {
	// Skip for now - this would require a complete test environment
	t.Skip("WIT integration test - requires complete test environment")

	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "wit-integration-test")
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
	var receivedMessages []api.Message
	router := func(ctx context.Context, msg api.Message) {
		receivedMessages = append(receivedMessages, msg)
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

	// Create manager
	manager, err := wasm2.NewManager(logger, kv, filepath.Join(tempDir, "plugins"), router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())

	// Build and load the health plugin
	justBuildHealth := exec.Command("just", "build-wasm-plugin", "health-wasm")
	if err := justBuildHealth.Run(); err != nil {
		t.Fatal(err)
	}

	healthWasm, err := os.ReadFile("build/wasm/health-wasm.wasm")
	if err != nil {
		t.Fatal(err)
	}

	// Load the health plugin
	caps := []api.Capability{
		{Method: "status", Description: "Get the health status of this WASM instance"},
	}

	err = manager.LoadPlugin(context.Background(), "health-plugin", healthWasm, caps)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for plugin to initialize
	time.Sleep(200 * time.Millisecond)

	// Test 1: Get plugin metadata
	metadata := manager.GetAllPluginMetadata()
	if len(metadata) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(metadata))
	}

	if metadata[0].ID != "health-plugin" {
		t.Errorf("expected health-plugin, got %s", metadata[0].ID)
	}

	// Test 2: Discover plugins with status capability
	statusPlugins := manager.DiscoverPlugins("status")
	if len(statusPlugins) != 1 {
		t.Errorf("expected 1 plugin with status capability, got %d", len(statusPlugins))
	}

	// Test 3: Route a message to the plugin
	testMsg := api.Message{
		ID:      "test-1",
		Method:  "status",
		Sender:  "test-client",
		Target:  "health-plugin",
		Payload: json.RawMessage(`{}`),
	}

	err = manager.RouteMessage(context.Background(), "health-plugin", testMsg)
	if err != nil {
		t.Fatal(err)
	}

	// Test 4: Get response from the plugin
	resp, err := manager.GetResponse(context.Background(), "health-plugin")
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

	if payload["status"] != "healthy" {
		t.Errorf("unexpected status: %s", payload["status"])
	}

	t.Log("WIT integration test completed successfully!")
}