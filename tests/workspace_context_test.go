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
	"github.com/james-nesbitt/alloy/pkg/storage"
	"github.com/james-nesbitt/alloy/pkg/wasm"
)

func TestWorkspaceContext(t *testing.T) {
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

	// Create message router
	router := func(ctx context.Context, msg api.Message) {
		// Mock router
	}

	// Create call function
	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		if msg.Target == "project" && msg.Method == "get_active" {
			resp := api.Message{
				ID:      msg.ID + "-resp",
				Type:    api.TypeResponse,
				Sender:  "project",
				Target:  "ai",
				Payload: json.RawMessage(`{"id":"test-proj","name":"Test Project","description":"Test project description"}`),
			}
			return resp, nil
		}
		return api.Message{}, nil
	}

	// Create manager
	manager, err := wasm.NewManager(logger, kv, filepath.Join(tempDir, "plugins"), nil, router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())

	// 1. Register a workspace
	ws := api.Workspace{
		ID:   "test-ws",
		Name: "Test Workspace",
		Path: "/test/path",
		Metadata: map[string]string{
			"env": "production",
		},
	}
	manager.RegisterWorkspace(ws)
	manager.SetActiveWorkspace(ws.ID)

	// 2. Load the AI plugin
	cwd, _ := os.Getwd()
	aiPath := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins/ai.wasm")
	wasmBytes, err := os.ReadFile(aiPath)
	if err != nil {
		t.Skip("AI plugin not built, skipping")
	}

	err = manager.LoadPlugin(context.Background(), "ai", wasmBytes, 128, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	// 3. Test AI query - it should include workspace context in its mock response
	reqID := "test-query"
	queryMsg := api.Message{
		ID:      reqID,
		Type:    api.TypeRequest,
		Sender:  "user",
		Target:  "ai",
		Method:  "query",
		Payload: json.RawMessage(`{"prompt":"hello context"}`),
	}

	err = manager.RouteMessage(context.Background(), "ai", queryMsg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := manager.GetResponse(context.Background(), "ai", reqID)
	if err != nil {
		t.Fatal(err)
	}

	var aiResp struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(resp.Payload, &aiResp); err != nil {
		t.Fatal(err)
	}

	t.Logf("AI Response: %s", aiResp.Response)

	// The AI plugin's getProjectContext function should have included the workspace name
	if !strings.Contains(aiResp.Response, "Test Workspace") {
		t.Errorf("AI response did not contain expected workspace context: %s", aiResp.Response)
	}

	if !strings.Contains(aiResp.Response, "/test/path") {
		t.Errorf("AI response did not contain expected workspace path: %s", aiResp.Response)
	}
}
