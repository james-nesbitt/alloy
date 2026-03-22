package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func TestBufferManagerOperations(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/wasm")
	bufferWasmPath := filepath.Join(buildDir, "buffer.wasm")

	if _, err := os.Stat(bufferWasmPath); os.IsNotExist(err) {
		t.Skip("buffer.wasm not found, run 'just build-plugins' first")
	}

	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "events", "type": "native"},
			{"id": "command-manager", "type": "native"},
			{"id": "kv", "type": "native"},
			{"id": "buffer", "type": "wasm", "path": bufferWasmPath, "memory_limit": 128},
		},
	}

	_, conn, collector, home := setupTestCore(t, "buffer_test", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	waitForPlugins(t, conn, collector, []string{"buffer"}, 30*time.Second)

	// 1. Test Create Buffer
	createReq, _ := json.Marshal(map[string]any{
		"name":      "test.txt",
		"type":      "ephemeral",
		"mime_type": "text/plain",
		"content":   []byte("Hello Alloy"),
		"metadata":  map[string]string{"author": "test-user"},
	})
	sendMsg(t, conn, api.Message{
		ID:      "buf-create-1",
		Sender:  "user",
		Target:  "buffer",
		Method:  "create",
		Payload: createReq,
	})
	resp := awaitResponse(t, collector, "buf-create-1-resp")
	var buffer struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp.Payload, &buffer); err != nil {
		t.Fatalf("failed to unmarshal buffer create response: %v", err)
	}
	bufID := buffer.ID
	if bufID == "" {
		t.Fatal("buffer ID is empty")
	}

	// 2. Test Read Buffer
	readReq, _ := json.Marshal(map[string]any{"id": bufID})
	sendMsg(t, conn, api.Message{
		ID:      "buf-read-1",
		Sender:  "user",
		Target:  "buffer",
		Method:  "read",
		Payload: readReq,
	})
	resp = awaitResponse(t, collector, "buf-read-1-resp")
	var readData struct {
		Content []byte `json:"content"`
		Size    int    `json:"size"`
	}
	json.Unmarshal(resp.Payload, &readData)
	if string(readData.Content) != "Hello Alloy" {
		t.Errorf("expected 'Hello Alloy', got '%s'", string(readData.Content))
	}

	// 3. Test Append Buffer
	appendReq, _ := json.Marshal(map[string]any{
		"id":      bufID,
		"content": []byte("! Append works."),
	})
	sendMsg(t, conn, api.Message{
		ID:      "buf-append-1",
		Sender:  "user",
		Target:  "buffer",
		Method:  "append",
		Payload: appendReq,
	})
	awaitResponse(t, collector, "buf-append-1-resp")

	// Verify append
	sendMsg(t, conn, api.Message{
		ID:      "buf-read-2",
		Sender:  "user",
		Target:  "buffer",
		Method:  "read",
		Payload: readReq,
	})
	resp = awaitResponse(t, collector, "buf-read-2-resp")
	json.Unmarshal(resp.Payload, &readData)
	if string(readData.Content) != "Hello Alloy! Append works." {
		t.Errorf("append failed, got '%s'", string(readData.Content))
	}

	// 4. Test Write (Partial Update)
	offset := 6
	writeReq, _ := json.Marshal(map[string]any{
		"id":      bufID,
		"content": []byte("World"),
		"offset":  &offset,
	})
	sendMsg(t, conn, api.Message{
		ID:      "buf-write-1",
		Sender:  "user",
		Target:  "buffer",
		Method:  "write",
		Payload: writeReq,
	})
	awaitResponse(t, collector, "buf-write-1-resp")

	// Verify write
	sendMsg(t, conn, api.Message{
		ID:      "buf-read-3",
		Sender:  "user",
		Target:  "buffer",
		Method:  "read",
		Payload: readReq,
	})
	resp = awaitResponse(t, collector, "buf-read-3-resp")
	json.Unmarshal(resp.Payload, &readData)
	if string(readData.Content) != "Hello World! Append works." {
		t.Errorf("write failed, got '%s'", string(readData.Content))
	}

	// 5. Test Metadata Update
	metaReq, _ := json.Marshal(map[string]any{
		"id": bufID,
		"metadata": map[string]string{
			"status": "ready",
		},
	})
	sendMsg(t, conn, api.Message{
		ID:      "buf-meta-1",
		Sender:  "user",
		Target:  "buffer",
		Method:  "set_metadata",
		Payload: metaReq,
	})
	awaitResponse(t, collector, "buf-meta-1-resp")

	// 6. Test List Buffers
	sendMsg(t, conn, api.Message{
		ID:     "buf-list-1",
		Sender: "user",
		Target: "buffer",
		Method: "list",
	})
	resp = awaitResponse(t, collector, "buf-list-1-resp")
	var listResp struct {
		Buffers []struct {
			ID       string         `json:"id"`
			Metadata map[string]any `json:"metadata"`
		} `json:"buffers"`
	}
	json.Unmarshal(resp.Payload, &listResp)
	if len(listResp.Buffers) != 1 {
		t.Fatalf("expected 1 buffer in list, got %d", len(listResp.Buffers))
	}
	if listResp.Buffers[0].Metadata["status"] != "ready" {
		t.Errorf("metadata not updated in list, got %v", listResp.Buffers[0].Metadata)
	}

	// 7. Test Delete
	delReq, _ := json.Marshal(map[string]any{"id": bufID})
	sendMsg(t, conn, api.Message{
		ID:      "buf-del-1",
		Sender:  "user",
		Target:  "buffer",
		Method:  "delete",
		Payload: delReq,
	})
	awaitResponse(t, collector, "buf-del-1-resp")

	// Verify deleted
	sendMsg(t, conn, api.Message{
		ID:     "buf-list-2",
		Sender: "user",
		Target: "buffer",
		Method: "list",
	})
	resp = awaitResponse(t, collector, "buf-list-2-resp")
	json.Unmarshal(resp.Payload, &listResp)
	if len(listResp.Buffers) != 0 {
		t.Errorf("buffer not deleted, still have %d", len(listResp.Buffers))
	}
}

func TestBufferSubscription(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/wasm")
	bufferWasmPath := filepath.Join(buildDir, "buffer.wasm")

	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "events", "type": "native"},
			{"id": "command-manager", "type": "native"},
			{"id": "buffer", "type": "wasm", "path": bufferWasmPath, "memory_limit": 128},
		},
	}

	_, conn, collector, home := setupTestCore(t, "sub_test", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	waitForPlugins(t, conn, collector, []string{"buffer"}, 30*time.Second)

	// 1. Create Buffer
	createReq, _ := json.Marshal(map[string]any{
		"name": "stream.log",
		"type": "stream",
	})
	sendMsg(t, conn, api.Message{
		ID:      "buf-create-1",
		Sender:  "user",
		Target:  "buffer",
		Method:  "create",
		Payload: createReq,
	})
	resp := awaitResponse(t, collector, "buf-create-1-resp")
	var buffer struct{ ID string }
	json.Unmarshal(resp.Payload, &buffer)
	bufID := buffer.ID

	// 2. Subscribe "other-user" to the buffer
	// We'll use our existing connection to simulate "other-user" if we send Sender="other-user"
	subReq, _ := json.Marshal(map[string]any{"id": bufID})
	sendMsg(t, conn, api.Message{
		ID:      "buf-sub-1",
		Sender:  "other-user",
		Target:  "buffer",
		Method:  "subscribe",
		Payload: subReq,
	})
	awaitResponse(t, collector, "buf-sub-1-resp")

	// 3. Append to buffer as "user"
	appendReq, _ := json.Marshal(map[string]any{
		"id":      bufID,
		"content": []byte("Log entry 1\n"),
	})
	sendMsg(t, conn, api.Message{
		ID:      "buf-app-1",
		Sender:  "user",
		Target:  "buffer",
		Method:  "append",
		Payload: appendReq,
	})
	awaitResponse(t, collector, "buf-app-1-resp")

	// 4. "other-user" should receive an event message directly
	evt, found := collector.Await(5*time.Second, func(m api.Message) bool {
		return m.Target == "other-user" && m.Type == "event"
	})
	if !found {
		t.Error("other-user never received the direct buffer update event")
	} else {
		t.Logf("Received direct event for other-user: %s", string(evt.Payload))
	}
}
