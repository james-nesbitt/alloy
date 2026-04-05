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

func TestCollaborativeKnowledgeGraph(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "knowledge-graph-test")
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

	// Create manager
	var manager *wasm.Manager
	router := func(ctx context.Context, msg api.Message) {}
	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		if msg.Target == "index" || msg.Target == "knowledge:search" {
			err := manager.RouteMessage(ctx, "index", msg)
			if err != nil {
				return api.Message{}, err
			}
			return manager.GetResponse(ctx, "index", msg.ID)
		}
		if msg.Target == "project" || msg.Target == "project:get_active" {
			content, _ := json.Marshal(map[string]string{
				"id":          "test-proj",
				"name":        "Test Project",
				"description": "A test project",
			})
			return api.Message{
				ID:      msg.ID + "-resp",
				Type:    api.TypeResponse,
				Sender:  "project",
				Target:  msg.Sender,
				Payload: content,
			}, nil
		}
		return api.Message{}, nil
	}

	manager, err = wasm.NewManager(logger, kv, filepath.Join(tempDir, "plugins"), nil, router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())

	// 1. Load the Index plugin
	cwd, _ := os.Getwd()
	indexPath := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins/index.wasm")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Skip("Index plugin not built, skipping")
	}
	err = manager.LoadPlugin(context.Background(), "index", indexBytes, 128, 100, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Load the AI plugin
	aiPath := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins/ai.wasm")
	aiBytes, err := os.ReadFile(aiPath)
	if err != nil {
		t.Skip("AI plugin not built, skipping")
	}
	err = manager.LoadPlugin(context.Background(), "ai", aiBytes, 128, 100, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	// 3. Ingest data into the Collaborative Knowledge Graph via user-1
	ingestMsg := api.Message{
		ID:     "ingest-1",
		Sender: "user-1",
		Target: "index",
		Method: "ingest",
		Payload: json.RawMessage(`{
			"path": "pkg/kernel/core.go",
			"content": "The kernel is the central part of the Alloy system. It handles message routing between plugins.",
			"tags": ["kernel", "routing"]
		}`),
	}
	manager.RouteMessage(context.Background(), "index", ingestMsg)
	manager.GetResponse(context.Background(), "index", "ingest-1")

	// 4. Query AI about the kernel - it should retrieve context from the index
	queryMsg := api.Message{
		ID:      "query-1",
		Sender:  "user-2",
		Target:  "ai",
		Method:  "query",
		Payload: json.RawMessage(`{"prompt":"tell me about the kernel"}`),
	}
	err = manager.RouteMessage(context.Background(), "ai", queryMsg)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := manager.GetResponse(context.Background(), "ai", "query-1")
	if err != nil {
		t.Fatal(err)
	}

	var aiResp struct {
		Response string `json:"response"`
	}
	json.Unmarshal(resp.Payload, &aiResp)

	t.Logf("AI Response: %s", aiResp.Response)

	// Check if AI response contains the indexed context
	if !strings.Contains(aiResp.Response, "Relevant Knowledge Graph Context") {
		t.Errorf("AI response did not contain Knowledge Graph context header")
	}
	if !strings.Contains(aiResp.Response, "central part of the Alloy system") {
		t.Errorf("AI response did not contain expected content from index")
	}
}
