package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func TestBufferCollaborativeEditing(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins")
	bufferWasmPath := filepath.Join(buildDir, "buffer.wasm")

	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "events", "type": "native"},
			{"id": "buffer", "type": "wasm", "path": bufferWasmPath},
		},
	}

	_, conn, collector, home := setupTestCore(t, "buffer_collaborative", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	waitForPlugins(t, conn, collector, []string{"buffer"}, 30*time.Second)

	// 1. Create a buffer
	createReq, _ := json.Marshal(map[string]any{
		"name":    "shared.txt",
		"content": []byte("Initial content"),
	})
	sendMsg(t, conn, api.Message{
		ID:      "buf-create",
		Sender:  "user-a",
		Target:  "buffer",
		Method:  "create",
		Payload: createReq,
	})
	resp := awaitResponse(t, collector, "buf-create-resp")
	var buf struct {
		ID      string
		Version int
	}
	json.Unmarshal(resp.Payload, &buf)
	if buf.ID == "" {
		t.Fatal("buffer ID is empty")
	}

	// 2. User A updates cursor
	cursorReq, _ := json.Marshal(map[string]any{
		"id":  buf.ID,
		"row": 10,
		"col": 5,
	})
	sendMsg(t, conn, api.Message{
		ID:      "cursor-a",
		Sender:  "user-a",
		Target:  "buffer",
		Method:  "update_cursor",
		Payload: cursorReq,
	})
	awaitResponse(t, collector, "cursor-a-resp")

	// 3. User B reads buffer and sees User A's cursor
	readReq, _ := json.Marshal(map[string]any{"id": buf.ID})
	sendMsg(t, conn, api.Message{
		ID:      "read-b",
		Sender:  "user-b",
		Target:  "buffer",
		Method:  "read",
		Payload: readReq,
	})
	resp = awaitResponse(t, collector, "read-b-resp")
	var readData struct {
		Cursors map[string]struct {
			Row  int    `json:"row"`
			Col  int    `json:"col"`
			User string `json:"user"`
		} `json:"cursors"`
		Version int `json:"version"`
	}
	json.Unmarshal(resp.Payload, &readData)

	cursorA, ok := readData.Cursors["user-a"]
	if !ok {
		t.Fatal("User A's cursor not found in response")
	}
	if cursorA.Row != 10 || cursorA.Col != 5 {
		t.Errorf("User A's cursor position incorrect: got (%d, %d), want (10, 5)", cursorA.Row, cursorA.Col)
	}

	// 4. Test Conflict Detection
	// User A makes a change (v0 -> v1)
	writeAReq, _ := json.Marshal(map[string]any{
		"id":           buf.ID,
		"base_version": 0,
		"content":      []byte("User A content"),
	})
	sendMsg(t, conn, api.Message{
		ID:      "write-a",
		Sender:  "user-a",
		Target:  "buffer",
		Method:  "write",
		Payload: writeAReq,
	})
	resp = awaitResponse(t, collector, "write-a-resp")
	var writeAResp struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}
	json.Unmarshal(resp.Payload, &writeAResp)
	if writeAResp.Status != "ok" || writeAResp.Version != 1 {
		t.Errorf("Write A failed: status=%s, version=%d", writeAResp.Status, writeAResp.Version)
	}

	// User B tries to update based on version 0 (v0 -> v1) but buffer is at v1
	writeBReq, _ := json.Marshal(map[string]any{
		"id":           buf.ID,
		"base_version": 0,
		"content":      []byte("User B conflicting content"),
	})
	sendMsg(t, conn, api.Message{
		ID:      "write-b",
		Sender:  "user-b",
		Target:  "buffer",
		Method:  "write",
		Payload: writeBReq,
	})
	resp = awaitResponse(t, collector, "write-b-resp")
	if resp.Method != "error" {
		t.Errorf("Expected conflict error from User B write, got %s", resp.Method)
	}
	var errData struct {
		Error string `json:"error"`
	}
	json.Unmarshal(resp.Payload, &errData)
	if errData.Error != "conflict_detected" {
		t.Errorf("Expected conflict_detected error, got %s", errData.Error)
	}

	// 5. Test Force Write (override conflict)
	forceReq, _ := json.Marshal(map[string]any{
		"id":      buf.ID,
		"content": []byte("Forced content"),
		"force":   true,
	})
	sendMsg(t, conn, api.Message{
		ID:      "write-force",
		Sender:  "user-b",
		Target:  "buffer",
		Method:  "write",
		Payload: forceReq,
	})
	resp = awaitResponse(t, collector, "write-force-resp")
	var forceResp struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}
	json.Unmarshal(resp.Payload, &forceResp)
	if forceResp.Status != "ok" || forceResp.Version != 2 {
		t.Errorf("Force write failed: status=%s, version=%d", forceResp.Status, forceResp.Version)
	}
}
