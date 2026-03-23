package tests

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
	"github.com/james-nesbitt/alloy/pkg/wasm"
)

func TestWITPlugins(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "wit-plugins-test")
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

	// Create message router
	var receivedMessages []api.Message
	router := func(ctx context.Context, msg api.Message) {
		receivedMessages = append(receivedMessages, msg)
	}

	// Create call function
	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		resp := api.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Method:  msg.Method,
			Sender:  msg.Target,
			Target:  msg.Sender,
			Payload: json.RawMessage(`{"result":"success"}`),
		}
		return resp, nil
	}

	// Create manager
	manager, err := wasm.NewManager(logger, kv, filepath.Join(tempDir, "plugins"), router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())

	// Build all plugins
	justBuildAll := exec.Command("just", "build-plugins")
	if err := justBuildAll.Run(); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	projectRoot := filepath.Dir(cwd)

	// Test each plugin
	testPlugins := []struct {
		name     string
		wasmFile string
		caps     []api.Capability
		tests    func(t *testing.T, manager *wasm.Manager)
	}{
		{
			name:     "health",
			wasmFile: filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/health.wasm"),
			caps:     []api.Capability{{Method: "status", Description: "Get health status"}},
			tests:    testHealthPlugin,
		},
		{
			name:     "buffer",
			wasmFile: filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/buffer.wasm"),
			caps: []api.Capability{
				{Method: "create", Description: "Create buffer"},
				{Method: "list", Description: "List buffers"},
			},
			tests: testBufferPlugin,
		},
		{
			name:     "chat",
			wasmFile: filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/chat.wasm"),
			caps: []api.Capability{
				{Method: "send", Description: "Send message"},
				{Method: "history", Description: "Get history"},
			},
			tests: testChatPlugin,
		},
		{
			name:     "ai",
			wasmFile: filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/ai.wasm"),
			caps: []api.Capability{
				{Method: "query", Description: "Query AI"},
				{Method: "config:get", Description: "Get AI config"},
			},
			tests: testAIPlugin,
		},
		{
			name:     "project",
			wasmFile: filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/project.wasm"),
			caps: []api.Capability{
				{Method: "create", Description: "Create project"},
				{Method: "list", Description: "List projects"},
			},
			tests: testProjectPlugin,
		},
		{
			name:     "iam",
			wasmFile: filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/iam.wasm"),
			caps: []api.Capability{
				{Method: "check", Description: "Check authorization"},
			},
			tests: testIAMPlugin,
		},
		{
			name:     "secrets",
			wasmFile: filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/secrets.wasm"),
			caps: []api.Capability{
				{Method: "store_secret", Description: "Store secret"},
				{Method: "get_secret", Description: "Get secret"},
			},
			tests: testSecretsPlugin,
		},
		{
			name:     "tasks",
			wasmFile: filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/tasks.wasm"),
			caps: []api.Capability{
				{Method: "create", Description: "Create task"},
				{Method: "list", Description: "List tasks"},
			},
			tests: testTasksPlugin,
		},
	}

	// Test each plugin
	for _, tp := range testPlugins {
		t.Run(tp.name, func(t *testing.T) {
			// Read the WASM file
			wasmBytes, err := os.ReadFile(tp.wasmFile)
			if err != nil {
				t.Fatal(err)
			}

			// Load the plugin
			err = manager.LoadPlugin(context.Background(), tp.name, wasmBytes, tp.caps)
			if err != nil {
				t.Fatal(err)
			}

			// Wait for plugin to initialize
			time.Sleep(200 * time.Millisecond)

			// Run plugin-specific tests
			tp.tests(t, manager)
		})
	}

	t.Log("All WIT plugin tests completed successfully!")
}

// Test functions for each plugin

func testHealthPlugin(t *testing.T, manager *wasm.Manager) {
	// Test health status
	testMsg := api.Message{
		ID:      "test-health",
		Method:  "status",
		Sender:  "test-client",
		Target:  "health",
		Payload: json.RawMessage(`{}`),
	}

	err := manager.RouteMessage(context.Background(), "health", testMsg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := manager.GetResponse(context.Background(), "health", "test-health")
	if err != nil {
		t.Fatal(err)
	}

	if resp.ID != "test-health-resp" {
		t.Errorf("unexpected response ID: %s", resp.ID)
	}

	var payload map[string]string
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatal(err)
	}

	if payload["status"] != "healthy" {
		t.Errorf("unexpected status: %s", payload["status"])
	}
}

func testBufferPlugin(t *testing.T, manager *wasm.Manager) {
	// Test buffer creation
	testMsg := api.Message{
		ID:      "test-buffer-create",
		Method:  "create",
		Sender:  "test-client",
		Target:  "buffer",
		Payload: json.RawMessage(`{"name":"test-buffer","type":"ephemeral"}`),
	}

	err := manager.RouteMessage(context.Background(), "buffer", testMsg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := manager.GetResponse(context.Background(), "buffer", testMsg.ID)
	if err != nil {
		t.Fatal(err)
	}

	if resp.ID != "test-buffer-create-resp" {
		t.Errorf("unexpected response ID: %s", resp.ID)
	}

	var buffer struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp.Payload, &buffer); err != nil {
		t.Fatal(err)
	}

	if buffer.ID == "" {
		t.Error("buffer ID should not be empty")
	}
}

func testChatPlugin(t *testing.T, manager *wasm.Manager) {
	// Test sending a message
	testMsg := api.Message{
		ID:      "test-chat-send",
		Method:  "send",
		Sender:  "test-client",
		Target:  "chat",
		Payload: json.RawMessage(`{"channel":"general","content":"Hello, world!"}`),
	}

	err := manager.RouteMessage(context.Background(), "chat", testMsg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := manager.GetResponse(context.Background(), "chat", testMsg.ID)
	if err != nil {
		t.Fatal(err)
	}

	if resp.ID != "test-chat-send-resp" {
		t.Errorf("unexpected response ID: %s", resp.ID)
	}
}

func testAIPlugin(t *testing.T, manager *wasm.Manager) {
	// Test getting AI config
	testMsg := api.Message{
		ID:      "test-ai-config",
		Method:  "config:get",
		Sender:  "test-client",
		Target:  "ai",
		Payload: json.RawMessage(`{}`),
	}

	err := manager.RouteMessage(context.Background(), "ai", testMsg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := manager.GetResponse(context.Background(), "ai", testMsg.ID)
	if err != nil {
		t.Fatal(err)
	}

	if resp.ID != "test-ai-config-resp" {
		t.Errorf("unexpected response ID: %s", resp.ID)
	}

	var config struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(resp.Payload, &config); err != nil {
		t.Fatal(err)
	}

	if config.Type == "" {
		t.Error("AI config type should not be empty")
	}
}

func testProjectPlugin(t *testing.T, manager *wasm.Manager) {
	// Test project creation
	testMsg := api.Message{
		ID:      "test-project-create",
		Method:  "create",
		Sender:  "test-client",
		Target:  "project",
		Payload: json.RawMessage(`{"name":"test-project","description":"Test project"}`),
	}

	err := manager.RouteMessage(context.Background(), "project", testMsg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := manager.GetResponse(context.Background(), "project", testMsg.ID)
	if err != nil {
		t.Fatal(err)
	}

	if resp.ID != "test-project-create-resp" {
		t.Errorf("unexpected response ID: %s", resp.ID)
	}

	var project struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp.Payload, &project); err != nil {
		t.Fatal(err)
	}

	if project.ID == "" {
		t.Error("project ID should not be empty")
	}
}

func testIAMPlugin(t *testing.T, manager *wasm.Manager) {
	// Test authorization check
	testMsg := api.Message{
		ID:      "test-iam-check",
		Method:  "check",
		Sender:  "test-client",
		Target:  "iam",
		Payload: json.RawMessage(`{"actor":"test-user","target":"chat","method":"send"}`),
	}

	err := manager.RouteMessage(context.Background(), "iam", testMsg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := manager.GetResponse(context.Background(), "iam", testMsg.ID)
	if err != nil {
		t.Fatal(err)
	}

	if resp.ID != "test-iam-check-resp" {
		t.Errorf("unexpected response ID: %s", resp.ID)
	}

	var result struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		t.Fatal(err)
	}

	// Should be allowed (guest role has default permissions)
	if !result.Allowed {
		t.Error("authorization should be allowed")
	}
}

func testSecretsPlugin(t *testing.T, manager *wasm.Manager) {
	// Test storing a secret
	testMsg := api.Message{
		ID:      "test-secrets-store",
		Method:  "store_secret",
		Sender:  "test-client",
		Target:  "secrets",
		Payload: json.RawMessage(`{"id":"test-secret","value":"secret-value"}`),
	}

	err := manager.RouteMessage(context.Background(), "secrets", testMsg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := manager.GetResponse(context.Background(), "secrets", testMsg.ID)
	if err != nil {
		t.Fatal(err)
	}

	if resp.ID != "test-secrets-store-resp" {
		t.Errorf("unexpected response ID: %s", resp.ID)
	}
}

func testTasksPlugin(t *testing.T, manager *wasm.Manager) {
	// Test creating a task
	testMsg := api.Message{
		ID:      "test-tasks-create",
		Method:  "create",
		Sender:  "test-client",
		Target:  "tasks",
		Payload: json.RawMessage(`{"title":"Test task","description":"Test description"}`),
	}

	err := manager.RouteMessage(context.Background(), "tasks", testMsg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := manager.GetResponse(context.Background(), "tasks", testMsg.ID)
	if err != nil {
		t.Fatal(err)
	}

	if resp.ID != "test-tasks-create-resp" {
		t.Errorf("unexpected response ID: %s", resp.ID)
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		t.Fatal(err)
	}

	if result.Status != "created" {
		t.Errorf("unexpected status: %s", result.Status)
	}
}
