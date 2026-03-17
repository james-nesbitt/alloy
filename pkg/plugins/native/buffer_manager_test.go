package native

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
)

func TestBufferManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	state := storage.NewMemoryStateStore()
	bm := NewBufferManager(logger, state)

	ctx := context.Background()

	// 1. Open a buffer
	openReq, _ := json.Marshal(map[string]string{
		"id":   "test-buffer",
		"type": "text",
	})
	msg := api.Message{
		ID:      "m1",
		Sender:  "user",
		Method:  "open",
		Payload: openReq,
	}

	resp, err := bm.HandleMessage(ctx, msg)
	if err != nil {
		t.Fatalf("failed to open buffer: %v", err)
	}

	var buf Buffer
	if err := json.Unmarshal(resp.Payload, &buf); err != nil {
		t.Fatalf("failed to unmarshal buffer response: %v", err)
	}

	if buf.ID != "test-buffer" {
		t.Errorf("expected buffer ID 'test-buffer', got %s", buf.ID)
	}

	// 2. Update the buffer
	updateReq, _ := json.Marshal(map[string]any{
		"id":      "test-buffer",
		"content": "Hello Alloy!",
		"version": 0,
	})
	msg = api.Message{
		ID:      "m2",
		Sender:  "user",
		Method:  "update",
		Payload: updateReq,
	}

	resp, err = bm.HandleMessage(ctx, msg)
	if err != nil {
		t.Fatalf("failed to update buffer: %v", err)
	}

	if err := json.Unmarshal(resp.Payload, &buf); err != nil {
		t.Fatalf("failed to unmarshal update response: %v", err)
	}

	if buf.Content != "Hello Alloy!" {
		t.Errorf("expected content 'Hello Alloy!', got %s", buf.Content)
	}
	if buf.Version != 1 {
		t.Errorf("expected version 1, got %d", buf.Version)
	}

	// 3. Conflict resolution test
	msg.ID = "m3"
	// updateReq with old version
	resp, err = bm.HandleMessage(ctx, msg)
	if err == nil {
		t.Error("expected conflict error, got nil")
	}

	// 4. List buffers
	msg = api.Message{
		ID:     "m4",
		Sender: "user",
		Method: "list",
	}
	resp, err = bm.HandleMessage(ctx, msg)
	if err != nil {
		t.Fatalf("failed to list buffers: %v", err)
	}

	var list []string
	json.Unmarshal(resp.Payload, &list)
	if len(list) != 1 || list[0] != "test-buffer" {
		t.Errorf("list mismatch: %v", list)
	}

	// 5. Persistence across shutdown
	bm.Shutdown(ctx)
	
	// Create a NEW manager with SAME state
	bm2 := NewBufferManager(logger, state)
	resp, err = bm2.HandleMessage(ctx, api.Message{
		ID:      "m5",
		Sender:  "user",
		Method:  "open",
		Payload: openReq,
	})
	if err != nil {
		t.Fatalf("failed to reopen buffer after shutdown: %v", err)
	}
	
	json.Unmarshal(resp.Payload, &buf)
	if buf.Content != "Hello Alloy!" {
		t.Errorf("persistence failed: expected content 'Hello Alloy!', got %s", buf.Content)
	}
}
