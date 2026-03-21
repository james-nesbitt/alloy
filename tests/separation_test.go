package tests

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

func TestSeparationRegistrationVsLoading(t *testing.T) {
	// 1. Create a manifest with a WASM plugin that DOES NOT EXIST on disk.
	// This proves that we can register it WITHOUT checking the file system yet.
	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "plugin-command-manager", "type": "native", "load_time": "boot"},
			{"id": "plugin-events", "type": "native", "load_time": "boot"},
			{
				"id": "missing-plugin", 
				"type": "wasm", 
				"path": "/tmp/non-existent.wasm", 
				"load_time": "lazy",
				"capabilities": []map[string]any{
					{"method": "phantom-call", "description": "I don't exist yet"},
				},
			},
		},
	}

	_, conn, collector, home := setupTestCore(t, "separation", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	// 2. Discover capabilities. "missing-plugin" should be visible because it's REGISTERED.
	t.Log("Discovering plugins via CommandManager...")
	found := false
	for i := 0; i < 5; i++ {
		sendMsg(t, conn, api.Message{
			ID:     "dis-1",
			Sender: "user",
			Target: "plugin-command-manager",
			Method: "discover",
		})
		
		resp, ok := collector.Await(1*time.Second, func(m api.Message) bool {
			return m.ID == "dis-1-resp"
		})
		
		if ok {
			var body struct { Targets []map[string]any `json:"targets" `}
			if err := json.Unmarshal(resp.Payload, &body); err != nil {
				t.Errorf("failed to unmarshal discovery response: %v", err)
				continue
			}
			for _, r := range body.Targets {
				if r["id"] == "missing-plugin" {
					found = true
					t.Log("SUCCESS: 'missing-plugin' registered successfully even though file is missing")
					break
				}
			}
		}
		if found { break }
		time.Sleep(500 * time.Millisecond)
	}

	if !found {
		t.Fatal("FAIL: 'missing-plugin' was never registered in metadata")
	}

	// 3. Now attempt to CALL the missing plugin. This should trigger the lazy-load and fail.
	t.Log("Attempting to call lazy-load for missing binary...")
	sendMsg(t, conn, api.Message{
		ID:     "call-1",
		Sender: "user",
		Target: "missing-plugin",
		Method: "phantom-call",
	})

	// We expect a response with an error from the Kernel
	errResp, ok := collector.Await(5*time.Second, func(m api.Message) bool {
		return m.ID == "call-1-resp" && m.Type == api.TypeResponse
	})

	if ok {
		var body map[string]any
		json.Unmarshal(errResp.Payload, &body)
		if errStr, ok := body["error"].(string); ok && errStr != "" {
			t.Logf("SUCCESS: Received expected error response on lazy-load failure: %s", string(errResp.Payload))
		} else {
			t.Errorf("FAIL: Received response but it did not contain an error: %s", string(errResp.Payload))
		}
	} else {
		t.Error("FAIL: Did not receive Response after calling missing lazy plugin")
	}
}
