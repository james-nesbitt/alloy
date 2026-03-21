package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

func TestWasmLoadMock(t *testing.T) {
	cwd, _ := os.Getwd()
	mockPath := filepath.Join(filepath.Dir(cwd), "build/wasm/mock.wasm")
	if _, err := os.Stat(mockPath); os.IsNotExist(err) {
		t.Skip("mock.wasm not found")
	}

	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "plugin-command-manager", "type": "native", "load_time": "boot"},
			{"id": "plugin-events", "type": "native", "load_time": "boot"},
			{"id": "plugin-mock", "type": "wasm", "path": mockPath, "load_time": "boot"},
		},
	}

	_, conn, collector, home := setupTestCore(t, "mock", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	waitForPlugins(t, conn, collector, []string{"plugin-mock"}, 30*time.Second)
}

func TestWasmLoadBulk(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/wasm")
	
	plugins := []string{"ai", "secrets", "health", "chat", "buffer"}
	wasmPlugins := []map[string]any{
		{"id": "plugin-command-manager", "type": "native", "load_time": "boot"},
		{"id": "plugin-events", "type": "native", "load_time": "boot"},
	}
	expectedIDs := []string{}

	for _, p := range plugins {
		path := filepath.Join(buildDir, p+".wasm")
		if _, err := os.Stat(path); err == nil {
			id := "plugin-" + p
			if p == "ai" { id = "plugin-ai-agent" }
			if p == "chat" { id = "plugin-chat" }
			if p == "buffer" { id = "plugin-buffer-manager" }

			wasmPlugins = append(wasmPlugins, map[string]any{
				"id":   id,
				"type": "wasm",
				"path": path,
				"load_time": "boot",
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

func TestWasmFunctionalSuite(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/wasm")

	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "plugin-command-manager", "type": "native", "load_time": "boot"},
			{"id": "plugin-events", "type": "native", "load_time": "boot"},
			{"id": "plugin-kv", "type": "native", "load_time": "boot"},
			{
				"id": "plugin-chat", 
				"type": "wasm", 
				"path": filepath.Join(buildDir, "chat.wasm"), 
				"load_time": "lazy",
				"capabilities": []map[string]string{{"method": "send"}},
			},
			{
				"id": "plugin-ai-agent", 
				"type": "wasm", 
				"path": filepath.Join(buildDir, "ai.wasm"), 
				"load_time": "lazy",
				"capabilities": []map[string]string{{"method": "query"}},
			},
			{
				"id": "plugin-secrets", 
				"type": "wasm", 
				"path": filepath.Join(buildDir, "secrets.wasm"), 
				"load_time": "lazy",
				"capabilities": []map[string]string{{"method": "store_secret"}},
			},
			{
				"id": "plugin-health", 
				"type": "wasm", 
				"path": filepath.Join(buildDir, "health.wasm"), 
				"load_time": "lazy",
				"capabilities": []map[string]string{{"method": "status"}},
			},
			{
				"id": "plugin-buffer-manager", 
				"type": "wasm", 
				"path": filepath.Join(buildDir, "buffer.wasm"), 
				"load_time": "lazy",
				"capabilities": []map[string]string{{"method": "open"}},
			},
		},
	}

	_, conn, collector, home := setupTestCore(t, "full", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	expected := []string{
		"plugin-chat", "plugin-ai-agent", "plugin-secrets", 
		"plugin-health", "plugin-buffer-manager",
	}
	
	// Check that lazy plugins are discoverable via metadata before loading
	waitForPlugins(t, conn, collector, expected, 30*time.Second)

	// 1. Verify Health (Loading it for the first time via request)
	sendMsg(t, conn, api.Message{
		ID:     "health-1",
		Sender: "user",
		Target: "plugin-health",
		Method: "status",
	})
	awaitResponse(t, collector, "health-1")
	
	// 2. Verify Secrets
	storeReq, _ := json.Marshal(map[string]string{"id": "db_pass", "value": "password123"})
	sendMsg(t, conn, api.Message{
		ID:      "secret-1",
		Sender:  "user",
		Target:  "plugin-secrets",
		Method:  "store_secret",
		Payload: storeReq,
	})
	awaitResponse(t, collector, "secret-1")

	getReq, _ := json.Marshal(map[string]string{"id": "db_pass"})
	sendMsg(t, conn, api.Message{
		ID:      "secret-2",
		Sender:  "user",
		Target:  "plugin-secrets",
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
		Target:  "plugin-buffer-manager",
		Method:  "create", // It was "open" in previous summary but the plugin has "create"
		Payload: openReq,
	})
	awaitResponse(t, collector, "buf-wasm-1")

	// 4. Verify Chat and AI Agent (Subscription and Reaction)
	// First, subscribe AI agent and user to chat events
	subReq, _ := json.Marshal(map[string]string{"topic": "chat:message"})
	sendMsg(t, conn, api.Message{
		ID:      "sub-ai",
		Sender:  "plugin-ai-agent",
		Target:  "plugin-events",
		Method:  "subscribe",
		Payload: subReq,
	})
	sendMsg(t, conn, api.Message{
		ID:      "sub-user",
		Sender:  "user",
		Target:  "plugin-events",
		Method:  "subscribe",
		Payload: subReq,
	})
	time.Sleep(200 * time.Millisecond) // wait for subs

	chatReq, _ := json.Marshal(map[string]string{
		"channel": "ai-test",
		"content": "ai: test bulk migration",
	})
	sendMsg(t, conn, api.Message{
		ID:      "chat-1",
		Sender:  "user",
		Target:  "plugin-chat",
		Method:  "send",
		Payload: chatReq,
	})

	// AI should react to the chat event
	chatEvt, ok := collector.Await(10*time.Second, func(m api.Message) bool {
		if m.Type == api.TypeEvent && m.Method == "chat:message" {
			var chatMsg struct { Sender string }
			json.Unmarshal(m.Payload, &chatMsg)
			if chatMsg.Sender == "plugin-ai-agent" {
				return true
			}
		}
		return false
	})
	if !ok {
		t.Error("never received AI agent response via Route in WASM")
	} else {
		var chatMsg struct { Content string }
		json.Unmarshal(chatEvt.Payload, &chatMsg)
		t.Logf("AI Response found: %s", chatMsg.Content)
	}
}
