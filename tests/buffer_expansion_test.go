package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func TestBufferExpansion(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins")
	bufferWasmPath := filepath.Join(buildDir, "buffer.wasm")

	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "events", "type": "native"},
			{"id": "command-manager", "type": "native"},
			{"id": "kv", "type": "native"},
			{"id": "buffer", "type": "wasm", "path": bufferWasmPath, "max_memory_mb": 256, "msg_per_second": 0},
		},
	}

	_, conn, collector, home := setupTestCore(t, "buffer_expansion", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	waitForPlugins(t, conn, collector, []string{"buffer"}, 30*time.Second)

	// 1. Test Indirect Buffers
	// Create base buffer
	createReq, _ := json.Marshal(map[string]any{
		"name":    "base.txt",
		"content": []byte("Hello Base"),
	})
	sendMsg(t, conn, api.Message{
		ID:      "base-create",
		Sender:  "user",
		Target:  "buffer",
		Method:  "create",
		Payload: createReq,
	})
	resp := awaitResponse(t, collector, "base-create-resp")
	var baseBuf struct{ ID string }
	json.Unmarshal(resp.Payload, &baseBuf)
	if baseBuf.ID == "" {
		t.Fatal("base buffer ID is empty")
	}

	// Create indirect buffer pointing to base
	indirectReq, _ := json.Marshal(map[string]any{
		"name":           "indirect.txt",
		"base_buffer_id": baseBuf.ID,
	})
	sendMsg(t, conn, api.Message{
		ID:      "indirect-create",
		Sender:  "user",
		Target:  "buffer",
		Method:  "create",
		Payload: indirectReq,
	})
	resp = awaitResponse(t, collector, "indirect-create-resp")
	var indirectBuf struct{ ID string }
	json.Unmarshal(resp.Payload, &indirectBuf)

	// Read from indirect buffer
	readReq, _ := json.Marshal(map[string]any{"id": indirectBuf.ID})
	sendMsg(t, conn, api.Message{
		ID:      "indirect-read-1",
		Sender:  "user",
		Target:  "buffer",
		Method:  "read",
		Payload: readReq,
	})
	resp = awaitResponse(t, collector, "indirect-read-1-resp")
	t.Logf("Indirect read raw payload: %s", string(resp.Payload))
	var readData struct {
		Content []byte `json:"content"`
		RootID  string `json:"root_id"`
	}
	json.Unmarshal(resp.Payload, &readData)
	t.Logf("Read result: root_id=%s, content=%v (len=%d)", readData.RootID, readData.Content, len(readData.Content))
	if string(readData.Content) != "Hello Base" {
		t.Errorf("indirect buffer read failed, expected 'Hello Base', got '%s' (len %d)", string(readData.Content), len(readData.Content))
	}

	// Double-indirect test
	doubleIndirectReq, _ := json.Marshal(map[string]any{
		"name":           "double.txt",
		"base_buffer_id": indirectBuf.ID,
	})
	sendMsg(t, conn, api.Message{
		ID:      "double-create",
		Sender:  "user",
		Target:  "buffer",
		Method:  "create",
		Payload: doubleIndirectReq,
	})
	resp = awaitResponse(t, collector, "double-create-resp")
	var doubleBuf struct{ ID string }
	json.Unmarshal(resp.Payload, &doubleBuf)

	// Write to double-indirect, verify base
	writeReq, _ := json.Marshal(map[string]any{
		"id":      doubleBuf.ID,
		"content": []byte("Shared Content"),
	})
	sendMsg(t, conn, api.Message{
		ID:      "double-write",
		Sender:  "user",
		Target:  "buffer",
		Method:  "write",
		Payload: writeReq,
	})
	awaitResponse(t, collector, "double-write-resp")

	// Verify base buf updated
	readBaseReq, _ := json.Marshal(map[string]any{"id": baseBuf.ID})
	sendMsg(t, conn, api.Message{
		ID:      "base-read-verify",
		Sender:  "user",
		Target:  "buffer",
		Method:  "read",
		Payload: readBaseReq,
	})
	resp = awaitResponse(t, collector, "base-read-verify-resp")
	var baseData struct {
		Content []byte `json:"content"`
	}
	json.Unmarshal(resp.Payload, &baseData)
	if string(baseData.Content) != "Shared Content" {
		t.Errorf("writing to double-indirect didn't update base, got '%s'", string(baseData.Content))
	}

	// 2. Test Stream Constraints (Max History)
	streamReq, _ := json.Marshal(map[string]any{
		"name": "stream",
		"type": "stream",
		"metadata": map[string]any{
			"max_history": 5,
		},
	})
	sendMsg(t, conn, api.Message{
		ID:      "stream-create",
		Sender:  "user",
		Target:  "buffer",
		Method:  "create",
		Payload: streamReq,
	})
	resp = awaitResponse(t, collector, "stream-create-resp")
	var streamBuf struct{ ID string }
	json.Unmarshal(resp.Payload, &streamBuf)

	// Append data exceeding max_history
	appendReq, _ := json.Marshal(map[string]any{
		"id":      streamBuf.ID,
		"content": []byte("1234567890"),
	})
	sendMsg(t, conn, api.Message{
		ID:      "stream-append-1",
		Sender:  "user",
		Target:  "buffer",
		Method:  "append",
		Payload: appendReq,
	})
	awaitResponse(t, collector, "stream-append-1-resp")

	// Read stream
	readStreamReq, _ := json.Marshal(map[string]any{"id": streamBuf.ID})
	sendMsg(t, conn, api.Message{
		ID:      "stream-read-verify",
		Sender:  "user",
		Target:  "buffer",
		Method:  "read",
		Payload: readStreamReq,
	})
	resp = awaitResponse(t, collector, "stream-read-verify-resp")
	var streamData struct {
		Content []byte `json:"content"`
	}
	json.Unmarshal(resp.Payload, &streamData)
	if string(streamData.Content) != "67890" {
		t.Errorf("stream max_history failed, expected '67890', got '%s'", string(streamData.Content))
	}

	// 3. Test Clear
	sendMsg(t, conn, api.Message{
		ID:      "stream-clear",
		Sender:  "user",
		Target:  "buffer",
		Method:  "clear",
		Payload: json.RawMessage(`{"id":"` + streamBuf.ID + `"}`),
	})
	awaitResponse(t, collector, "stream-clear-resp")

	// Verify clear
	sendMsg(t, conn, api.Message{
		ID:      "stream-read-empty",
		Sender:  "user",
		Target:  "buffer",
		Method:  "read",
		Payload: readStreamReq,
	})
	resp = awaitResponse(t, collector, "stream-read-empty-resp")
	json.Unmarshal(resp.Payload, &readData)
	if len(readData.Content) != 0 {
		t.Errorf("clear failed, expected 0 bytes, got %d", len(readData.Content))
	}
}
