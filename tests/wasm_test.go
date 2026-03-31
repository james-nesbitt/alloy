package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func TestWasmLoadMock(t *testing.T) {
	cwd, _ := os.Getwd()
	mockPath := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins/mock.wasm")
	if _, err := os.Stat(mockPath); os.IsNotExist(err) {
		t.Skip("mock.wasm not found")
	}

	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "mock", "type": "wasm", "path": mockPath, "load_time": "boot"},
		},
	}

	_, conn, collector, home := setupTestCore(t, "mock", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	waitForPlugins(t, conn, collector, []string{"mock"}, 30*time.Second)
}

func TestWasmLoadBulk(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins")

	plugins := []string{"ai", "secrets", "health", "chat", "buffer"}
	wasmPlugins := []map[string]any{}
	expectedIDs := []string{}

	for _, p := range plugins {
		path := filepath.Join(buildDir, p+".wasm")
		if _, err := os.Stat(path); err == nil {
			id := p
			caps := []map[string]string{}
			if p == "chat" {
				caps = append(caps, map[string]string{"method": "chat:send", "description": "Send chat"})
			}
			if p == "ai" {
				caps = append(caps, map[string]string{"method": "ai:query", "description": "Query AI"})
			}

			wasmPlugins = append(wasmPlugins, map[string]any{
				"id":           id,
				"type":         "wasm",
				"path":         path,
				"load_time":    "boot",
				"capabilities": caps,
			})
			expectedIDs = append(expectedIDs, id)
		}
	}

	manifest := map[string]any{"plugins": wasmPlugins}
	_, conn, collector, home := setupTestCore(t, "bulk", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	waitForPlugins(t, conn, collector, expectedIDs, 120*time.Second)
}

func TestSeparationRegistrationVsLoading(t *testing.T) {
	// 1. Create a manifest with a WASM plugin that DOES NOT EXIST on disk.
	// This proves that we can register it WITHOUT checking the file system yet.
	manifest := map[string]any{
		"plugins": []map[string]any{
			{
				"id":        "missing-plugin",
				"type":      "wasm",
				"path":      "/tmp/non-existent.wasm",
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
			Target: "command-manager",
			Method: "discover",
		})

		resp, ok := collector.Await(1*time.Second, func(m api.Message) bool {
			return m.ID == "dis-1-resp"
		})

		if ok {
			var body struct {
				Targets []map[string]any `json:"targets" `
			}
			if err := json.Unmarshal(resp.Payload, &body); err != nil {
				t.Logf("failed to unmarshal discovery response: %v", err)
				continue
			}
			t.Logf("Discovery result: %d targets found", len(body.Targets))
			for _, r := range body.Targets {
				t.Logf("  Found target: %s (type=%s)", r["id"], r["type"])
				if r["id"] == "missing-plugin" {
					found = true
					t.Log("SUCCESS: 'missing-plugin' registered successfully even though file is missing")
					break
				}
			}
		}
		if found {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !found {
		t.Fatal("FAIL: 'missing-plugin' was never registered in metadata")
	}

	// 3. Now attempt to CALL the missing plugin. This should trigger the lazy-load and fail.
	t.Log("Attempting to call lazy-load for missing binary...")
	sendMsg(t, conn, api.Message{
		ID:     "call-1",
		Type:   api.TypeRequest,
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

func TestWasmFunctionalSuite(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins")

	manifest := map[string]any{
		"plugins": []map[string]any{
			{
				"id":           "chat",
				"type":         "wasm",
				"path":         filepath.Join(buildDir, "chat.wasm"),
				"load_time":    "lazy",
				"capabilities": []map[string]string{{"method": "chat:send"}},
			},
			{
				"id":           "ai",
				"type":         "wasm",
				"path":         filepath.Join(buildDir, "ai.wasm"),
				"load_time":    "boot",
				"capabilities": []map[string]string{{"method": "ai:query"}},
			},
			{
				"id":           "secrets",
				"type":         "wasm",
				"path":         filepath.Join(buildDir, "secrets.wasm"),
				"load_time":    "lazy",
				"capabilities": []map[string]string{{"method": "store_secret"}},
			},
			{
				"id":           "test-health",
				"type":         "wasm",
				"path":         filepath.Join(buildDir, "health.wasm"),
				"load_time":    "lazy",
				"capabilities": []map[string]string{{"method": "health:status"}},
			},
			{
				"id":           "buffer",
				"type":         "wasm",
				"path":         filepath.Join(buildDir, "buffer.wasm"),
				"load_time":    "lazy",
				"capabilities": []map[string]string{{"method": "buffer:create"}},
			},
		},
	}

	_, conn, collector, home := setupTestCore(t, "full", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	expected := []string{
		"chat", "ai", "secrets",
		"test-health", "buffer",
	}

	// Check that lazy plugins are discoverable via metadata before loading
	waitForPlugins(t, conn, collector, expected, 60*time.Second)

	// 1. Verify Health (Loading it for the first time via request)
	sendMsg(t, conn, api.Message{
		ID:     "health-1",
		Type:   api.TypeRequest,
		Sender: "user",
		Target: "test-health",
		Method: "status",
	})
	awaitResponse(t, collector, "health-1")

	// 2. Verify Secrets
	storeReq, _ := json.Marshal(map[string]string{"id": "db_pass", "value": "password123"})
	sendMsg(t, conn, api.Message{
		ID:      "secret-1",
		Sender:  "user",
		Target:  "secrets",
		Method:  "store_secret",
		Payload: storeReq,
	})
	awaitResponse(t, collector, "secret-1")

	getReq, _ := json.Marshal(map[string]string{"id": "db_pass"})
	sendMsg(t, conn, api.Message{
		ID:      "secret-2",
		Sender:  "user",
		Target:  "secrets",
		Method:  "get_secret",
		Payload: getReq,
	})
	resp := awaitResponse(t, collector, "secret-2")
	if !strings.Contains(string(resp.Payload), "password123") {
		t.Errorf("secret mismatch, got: %s", string(resp.Payload))
	}

	// 3. Verify Buffer Manager
	openReq, _ := json.Marshal(map[string]string{
		"id":   "doc-1",
		"type": "text",
	})
	sendMsg(t, conn, api.Message{
		ID:      "buf-wasm-1",
		Type:    api.TypeRequest,
		Sender:  "user",
		Target:  "buffer",
		Method:  "create", // It was "open" in previous summary but the plugin has "create"
		Payload: openReq,
	})
	awaitResponse(t, collector, "buf-wasm-1")

	// 4. Verify Chat and AI Agent (Subscription and Reaction)
	// First, subscribe user to chat events (AI agent does it itself on start)
	subReq, _ := json.Marshal(map[string]string{"topic": "chat:message"})
	sendMsg(t, conn, api.Message{
		ID:      "sub-user",
		Sender:  "user",
		Target:  "events",
		Method:  "subscribe",
		Payload: subReq,
	})
	time.Sleep(2 * time.Second) // Wait longer for subs to process in WASM

	chatReq, _ := json.Marshal(map[string]string{
		"channel": "ai-test",
		"content": "ai: test bulk migration",
	})
	sendMsg(t, conn, api.Message{
		ID:      "chat-1",
		Sender:  "user",
		Target:  "chat",
		Method:  "send",
		Payload: chatReq,
	})

	// AI should react to the chat event
	chatEvt, ok := collector.Await(60*time.Second, func(m api.Message) bool {
		if m.Type == api.TypeEvent && m.Method == "chat:message" {
			var chatMsg struct{ Sender string }
			json.Unmarshal(m.Payload, &chatMsg)
			if chatMsg.Sender == "ai" {
				return true
			}
		}
		return false
	})
	if !ok {
		t.Error("never received AI agent response via Route in WASM")
	} else {
		var chatMsg struct{ Content string }
		json.Unmarshal(chatEvt.Payload, &chatMsg)
		t.Logf("AI Response found: %s", chatMsg.Content)
	}
}
