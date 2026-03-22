package wasm_test

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
	"github.com/jnesbitt/alloy-go/pkg/wasm"
)

func TestWITRuntime(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "wasm2-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Set up logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Set up storage
	kv, err := storage.NewBadgerStore(filepath.Join(tempDir, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()

	// Create message router
	router := func(ctx context.Context, msg api.Message) {
		logger.Info("routed message", "method", msg.Method, "sender", msg.Sender, "target", msg.Target)
	}

	// Create call function
	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		logger.Info("plugin called", "method", msg.Method, "sender", msg.Sender)
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

	// Start monitor
	manager.StartMonitor(context.Background(), 30*time.Second)

	// Test would continue with actual WASM loading and interaction
	// For now, just verify the manager was created successfully
	logger.Info("WIT manager created successfully")
}