package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func TestBufferPersistence(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins")
	bufferWasmPath := filepath.Join(buildDir, "buffer.wasm")

	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "command-manager", "type": "native"},
			{"id": "events", "type": "native"},
			{"id": "kv", "type": "native"},
			{"id": "buffer", "type": "wasm", "path": bufferWasmPath, "memory_limit": 128},
		},
	}

	_, conn, collector, home := setupTestCore(t, "persistence_test", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	waitForPlugins(t, conn, collector, []string{"buffer"}, 30*time.Second)

	// 1. Create a buffer
	createReq, _ := json.Marshal(map[string]any{
		"name":    "persist.txt",
		"content": []byte("Persistent Content"),
	})
	sendMsg(t, conn, api.Message{
		ID:      "create-req",
		Sender:  "user",
		Target:  "buffer",
		Method:  "create",
		Payload: createReq,
	})
	resp := awaitResponse(t, collector, "create-req-resp")
	var buffer struct{ ID string }
	json.Unmarshal(resp.Payload, &buffer)
	bufID := buffer.ID

	// 2. Save the buffer
	saveReq, _ := json.Marshal(map[string]any{"id": bufID})
	sendMsg(t, conn, api.Message{
		ID:      "save-req",
		Sender:  "user",
		Target:  "buffer",
		Method:  "save",
		Payload: saveReq,
	})
	resp = awaitResponse(t, collector, "save-req-resp")
	var saveResp map[string]string
	json.Unmarshal(resp.Payload, &saveResp)
	if saveResp["status"] != "ok" {
		t.Fatalf("save failed: %v", saveResp)
	}

	// Wait a moment for KV operations to complete (async in kernel)
	time.Sleep(200 * time.Millisecond)

	// 3. Delete from memory (unload)
	delReq, _ := json.Marshal(map[string]any{"id": bufID})
	sendMsg(t, conn, api.Message{
		ID:      "del-req",
		Sender:  "user",
		Target:  "buffer",
		Method:  "unload",
		Payload: delReq,
	})
	awaitResponse(t, collector, "del-req-resp")

	// Verify it's gone from memory
	sendMsg(t, conn, api.Message{
		ID:     "list-req-1",
		Sender: "user",
		Target: "buffer",
		Method: "list",
	})
	resp = awaitResponse(t, collector, "list-req-1-resp")
	var listResp struct {
		Buffers []any `json:"buffers"`
	}
	json.Unmarshal(resp.Payload, &listResp)
	if len(listResp.Buffers) != 0 {
		t.Errorf("buffer not deleted from memory, still have %d", len(listResp.Buffers))
	}

	// 4. Load from KV
	sendMsg(t, conn, api.Message{
		ID:     "load-req",
		Sender: "user",
		Target: "buffer",
		Method: "load",
	})
	awaitResponse(t, collector, "load-req-resp")

	// Wait for async loading to finish
	time.Sleep(500 * time.Millisecond)

	// 5. Verify it's back
	sendMsg(t, conn, api.Message{
		ID:     "list-req-2",
		Sender: "user",
		Target: "buffer",
		Method: "list",
	})
	resp = awaitResponse(t, collector, "list-req-2-resp")
	json.Unmarshal(resp.Payload, &listResp)
	if len(listResp.Buffers) == 0 {
		t.Fatal("buffer failed to reload from KV")
	}

	// 6. Verify content is back too
	readReq, _ := json.Marshal(map[string]any{"id": bufID})
	sendMsg(t, conn, api.Message{
		ID:      "read-req",
		Sender:  "user",
		Target:  "buffer",
		Method:  "read",
		Payload: readReq,
	})
	resp = awaitResponse(t, collector, "read-req-resp")
	var readData struct {
		Content []byte `json:"content"`
	}
	json.Unmarshal(resp.Payload, &readData)
	if string(readData.Content) != "Persistent Content" {
		t.Errorf("content failed to reload, expected 'Persistent Content', got '%s'", string(readData.Content))
	}
}
