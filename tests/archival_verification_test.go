package tests

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
	"github.com/james-nesbitt/alloy/pkg/storage/history"
	"github.com/james-nesbitt/alloy/pkg/wasm"
)

func TestWorkspaceArchivalSerial(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "archival-serial")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	storagePath := filepath.Join(tempDir, "storage")
	kv, err := storage.NewFileStateStore(storagePath)
	if err != nil {
		t.Fatal(err)
	}

	historyPath := filepath.Join(tempDir, "history")
	hStore, err := history.NewStore(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer hStore.Close()

	historyManager := kernel.NewHistoryManager(logger, hStore)

	pluginDir := filepath.Join(tempDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	var manager *wasm.Manager

	router := func(ctx context.Context, msg api.Message) {
		manager.RouteMessage(ctx, msg.Target, msg)
	}

	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		if msg.Target == "history" {
			return historyManager.HandleMessage(ctx, msg)
		}
		if msg.Target == "command-manager" {
			if msg.Method == "command-manager:discover" || msg.Method == "service:discovery" || msg.Method == "list" {
				payload, _ := json.Marshal(map[string]interface{}{
					"targets": []api.Registration{
						{ID: "project", Capabilities: []api.Capability{{Method: "project:archive"}}},
						{ID: "librarian", Capabilities: []api.Capability{{Method: "librarian:index-archive"}}},
						{ID: "buffer"},
					},
				})
				return api.Message{ID: msg.ID + "-resp", Type: api.TypeResponse, Method: msg.Method, Sender: "command-manager", Payload: payload}, nil
			}
			return api.Message{ID: msg.ID + "-resp", Type: api.TypeResponse, Method: msg.Method, Sender: "command-manager"}, nil
		}
		return manager.Call(ctx, msg.Target, msg)
	}

	manager, err = wasm.NewManager(logger, kv, pluginDir, nil, router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())

	cwd, _ := os.Getwd()
	distDir := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins")

	for _, id := range []string{"project", "librarian"} {
		wasmBytes, _ := os.ReadFile(filepath.Join(distDir, id+".wasm"))
		manager.LoadPlugin(context.Background(), id, wasmBytes, 128, 100, nil, false)
	}

	time.Sleep(1 * time.Second)

	// Step 1: Create a project
	t.Log("Step 1: Creating project")
	_, err = manager.Call(context.Background(), "project", api.Message{
		ID:     "init-1",
		Method: "project:create",
		Sender: "test",
		Target: "project",
		Payload: []byte(`{"name": "test-project", "description": "test"}`),
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Step 2: Trigger Archival
	t.Log("Step 2: Archiving")
	resp, err := manager.Call(context.Background(), "project", api.Message{
		ID:     "arch-1",
		Method: "project:archive",
		Sender: "test",
		Target: "project",
	})
	if err != nil {
		t.Fatalf("archival failed: %v", err)
	}

	var archRes struct {
		Filename string `json:"filename"`
	}
	json.Unmarshal(resp.Payload, &archRes)
	t.Logf("Archive created: %s", archRes.Filename)

	// Step 3: Verify restoration
	t.Log("Step 3: Restoring")
	restoreReq, _ := json.Marshal(map[string]string{"filename": archRes.Filename})
	_, err = manager.Call(context.Background(), "project", api.Message{
		ID:      "rest-1",
		Method:  "project:restore",
		Sender:  "test",
		Target:  "project",
		Payload: restoreReq,
	})
	if err != nil {
		t.Fatalf("restoration failed: %v", err)
	}

	t.Log("Success: Workspace Archival Loop Verified (Core)")
}
