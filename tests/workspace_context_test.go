package tests

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/kernel"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

func TestUnifiedProjectContext(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "workspace-context-test")
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
	k, err := kernel.NewWITKernel(logger, kv, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Shutdown(context.Background())

	cwd, _ := os.Getwd()
	projectRoot := filepath.Dir(cwd)

	// Build plugins
	// Note: We assume they are built or we trigger it via just
	
	// Load Project plugin
	projectWasm, err := os.ReadFile(filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/project.wasm"))
	if err != nil {
		t.Fatalf("failed to read project.wasm (run 'just build-plugin project'): %v", err)
	}
	err = k.RegisterWASMPlugin("project", projectWasm, []api.Capability{
		{Method: "create", Description: "Create project"},
		{Method: "open", Description: "Open project"},
		{Method: "get_active", Description: "Get active project"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Load AI plugin
	aiWasm, err := os.ReadFile(filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/ai.wasm"))
	if err != nil {
		t.Fatalf("failed to read ai.wasm (run 'just build-plugin ai'): %v", err)
	}
	err = k.RegisterWASMPlugin("ai", aiWasm, []api.Capability{
		{Method: "query", Description: "Query AI"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for plugins to initialize
	time.Sleep(1 * time.Second)

	frontendCh := make(chan api.Message, 100)
	k.RegisterFrontend("test-user", frontendCh)

	// Helper to wait for a specific response
	waitForResponse := func(requestID string, timeout time.Duration) api.Message {
		start := time.Now()
		for time.Since(start) < timeout {
			select {
			case msg := <-frontendCh:
				// WIT responses have ID as requestID + "-resp"
				if msg.ID == requestID+"-resp" || msg.ID == requestID {
					return msg
				}
				// Log other messages we might be getting
				t.Logf("Skipping message: ID=%s, Method=%s", msg.ID, msg.Method)
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		t.Fatalf("Timeout waiting for response to %s", requestID)
		return api.Message{}
	}

	// 1. Create a project
	createProjMsg := api.Message{
		ID:      "req-create-proj",
		Type:    api.TypeRequest,
		Method:  "create",
		Sender:  "test-user",
		Target:  "project",
		Payload: json.RawMessage(`{"name":"test-project","description":"a project for testing context"}`),
	}
	k.RouteMessage(context.Background(), createProjMsg)

	resp := waitForResponse("req-create-proj", 10*time.Second)
	t.Logf("Create Project Response: %s", string(resp.Payload))
	
	var proj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp.Payload, &proj); err != nil {
		t.Fatal(err)
	}
	projectID := proj.ID
	t.Logf("Created project: %s", projectID)

	// 2. Open the project
	openProjMsg := api.Message{
		ID:      "req-open-proj",
		Type:    api.TypeRequest,
		Method:  "open",
		Sender:  "test-user",
		Target:  "project",
		Payload: json.RawMessage(`{"id":"` + projectID + `"}`),
	}
	k.RouteMessage(context.Background(), openProjMsg)
	
	resp = waitForResponse("req-open-proj", 5*time.Second)
	t.Logf("Open Project Response: %s", string(resp.Payload))

	// 3. Register a workspace in the Host-side registry
	workspace := api.Workspace{
		ID:   "ws-1",
		Name: "Test Workspace",
		Path: "/tmp/test-workspace",
	}
	k.RegisterWorkspace(workspace)
	k.SetActiveWorkspace(workspace.ID)

	// 4. Query the AI plugin and verify it sees the context
	queryMsg := api.Message{
		ID:      "req-query",
		Type:    api.TypeRequest,
		Method:  "query",
		Sender:  "test-user",
		Target:  "ai",
		Payload: json.RawMessage(`{"prompt":"what is the context?"}`),
	}
	k.RouteMessage(context.Background(), queryMsg)

	resp = waitForResponse("req-query", 15*time.Second)
	t.Logf("AI Query Response: %s", string(resp.Payload))
	
	var aiResp struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(resp.Payload, &aiResp); err != nil {
		t.Fatal(err)
	}
	
	// Verify AI response contains both workspace and project info
	if !strings.Contains(aiResp.Response, "Test Workspace") {
		t.Errorf("AI response missing workspace name context: %s", aiResp.Response)
	}
	if !strings.Contains(aiResp.Response, "/tmp/test-workspace") {
		t.Errorf("AI response missing workspace path context: %s", aiResp.Response)
	}
	if !strings.Contains(aiResp.Response, "test-project") {
		t.Errorf("AI response missing project name context: %s", aiResp.Response)
	}
	if !strings.Contains(aiResp.Response, "a project for testing context") {
		t.Errorf("AI response missing project description context: %s", aiResp.Response)
	}

	t.Log("Unified Project Context test passed!")
}
