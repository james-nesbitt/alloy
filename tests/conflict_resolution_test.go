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
	"github.com/james-nesbitt/alloy/pkg/storage"
	"github.com/james-nesbitt/alloy/pkg/wasm"
)

func TestBufferConflictResolution(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "conflict-res-test")
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
	router := func(ctx context.Context, msg api.Message) {}
	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		return api.Message{}, nil
	}

	manager, err := wasm.NewManager(logger, kv, filepath.Join(tempDir, "plugins"), nil, router, call)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())

	// 1. Load the buffer plugin
	cwd, _ := os.Getwd()
	bufferPath := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins/buffer.wasm")
	wasmBytes, err := os.ReadFile(bufferPath)
	if err != nil {
		t.Skip("Buffer plugin not built, skipping")
	}

	err = manager.LoadPlugin(context.Background(), "buffer", wasmBytes, 128, 100, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	// 2. Create a buffer with initial content: "ABC"
	// "ABC" in base64 is "QUJD"
	createMsg := api.Message{
		ID:      "create-1",
		Sender:  "user-1",
		Target:  "buffer",
		Method:  "create",
		Payload: json.RawMessage(`{"name":"test","content":"QUJD"}`),
	}
	manager.RouteMessage(context.Background(), "buffer", createMsg)
	resp, _ := manager.GetResponse(context.Background(), "buffer", "create-1")
	var buffer struct {
		ID string `json:"id"`
	}
	json.Unmarshal(resp.Payload, &buffer)
	bufferID := buffer.ID

	// 3. User 1 inserts "X" at 0.
	// "X" in base64 is "WA=="
	write1 := api.Message{
		ID:      "write-1",
		Sender:  "user-1",
		Target:  "buffer",
		Method:  "write",
		Payload: json.RawMessage(`{"id":"` + bufferID + `","base_version":0,"content":"WA==","offset":0,"action":"insert"}`),
	}
	manager.RouteMessage(context.Background(), "buffer", write1)
	manager.GetResponse(context.Background(), "buffer", "write-1")

	// 4. User 2 inserts "Y" at 3 (end of original ABC) based on v0.
	// "Y" in base64 is "WQ=="
	// Transformation should move this to offset 4.
	write2 := api.Message{
		ID:      "write-2",
		Sender:  "user-2",
		Target:  "buffer",
		Method:  "write",
		Payload: json.RawMessage(`{"id":"` + bufferID + `","base_version":0,"content":"WQ==","offset":3,"action":"insert"}`),
	}
	manager.RouteMessage(context.Background(), "buffer", write2)
	manager.GetResponse(context.Background(), "buffer", "write-2")

	// 5. Verify combined content
	readMsg := api.Message{
		ID:      "read-1",
		Sender:  "user-1",
		Target:  "buffer",
		Method:  "read",
		Payload: json.RawMessage(`{"id":"` + bufferID + `"}`),
	}
	manager.RouteMessage(context.Background(), "buffer", readMsg)
	readResp, _ := manager.GetResponse(context.Background(), "buffer", "read-1")
	var fullBuffer struct {
		Content []byte `json:"content"`
	}
	json.Unmarshal(readResp.Payload, &fullBuffer)

	expected := "XABCY"
	if string(fullBuffer.Content) != expected {
		t.Errorf("Expected content '%s', got '%s'", expected, string(fullBuffer.Content))
	}
}
