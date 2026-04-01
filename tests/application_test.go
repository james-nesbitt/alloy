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

type ChatMessage struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

func TestApplicationPlugins(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins")

	manifest := map[string]any{
		"plugins": []map[string]any{
			{
				"id":           "buffer",
				"type":         "wasm",
				"path":         filepath.Join(buildDir, "buffer.wasm"),
				"capabilities": []map[string]string{{"method": "buffer:create"}, {"method": "buffer:read"}},
			},
			{
				"id":           "project",
				"type":         "wasm",
				"path":         filepath.Join(buildDir, "project.wasm"),
				"capabilities": []map[string]string{{"method": "project:create"}, {"method": "project:open"}},
			},
			{
				"id":           "chat",
				"type":         "wasm",
				"path":         filepath.Join(buildDir, "chat.wasm"),
				"capabilities": []api.Capability{{Method: "chat:send"}},
			},
			{
				"id":            "ai",
				"type":          "wasm",
				"path":          filepath.Join(buildDir, "ai.wasm"),
				"max_memory_mb": 256,
				"capabilities":  []api.Capability{{Method: "ai:query"}},
			},
		},
	}

	_, conn, collector, home := setupTestCore(t, "app-test", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	t.Log("Polling for WASM plugins to register...")
	expected := []string{
		"chat", "ai", "buffer", "project",
	}
	waitForPlugins(t, conn, collector, expected, 30*time.Second)
	t.Log("All WASM plugins registered")

	// 1. Subscribe to chat events for the test connection and the AI agent
	subReq, _ := json.Marshal(map[string]string{"topic": "chat:message"})
	// Test connection subscription
	sendMsg(t, conn, api.Message{
		ID:      "sub-test",
		Sender:  "user-1",
		Target:  "events",
		Method:  "subscribe",
		Payload: subReq,
	})
	awaitResponse(t, collector, "sub-test-resp")

	// 2. Clear old collector messages before test
	time.Sleep(100 * time.Millisecond)

	// 2.5. Setup an active project
	projReq, _ := json.Marshal(map[string]string{
		"name":        "Test Project",
		"description": "Integration Test Project",
	})
	sendMsg(t, conn, api.Message{
		ID:      "proj-create-1",
		Sender:  "user-1",
		Target:  "project",
		Method:  "create",
		Payload: projReq,
	})
	resp := awaitResponse(t, collector, "proj-create-1-resp")
	var project struct {
		ID string `json:"id"`
	}
	json.Unmarshal(resp.Payload, &project)

	openReq, _ := json.Marshal(map[string]string{"id": project.ID})
	sendMsg(t, conn, api.Message{
		ID:      "proj-open-1",
		Sender:  "user-1",
		Target:  "project",
		Method:  "open",
		Payload: openReq,
	})
	awaitResponse(t, collector, "proj-open-1-resp")

	// 2.6. Wait a moment for all plugins to finish OnInit/OnStart (including subscriptions)
	time.Sleep(500 * time.Millisecond)

	// 3. User sends a message that should trigger AI response
	chatReq, _ := json.Marshal(map[string]string{
		"channel": "test-channel",
		"content": "AI: hello world",
	})
	sendMsg(t, conn, api.Message{
		ID:      "chat-1",
		Sender:  "user-1",
		Target:  "chat",
		Method:  "send",
		Payload: chatReq,
	})
	awaitResponse(t, collector, "chat-1-resp")

	// 4. Verify AI Response via event bus
	aiEvt, found := collector.Await(20*time.Second, func(m api.Message) bool {
		if m.Method == "chat:message" && m.Type == "event" {
			var chatMsg ChatMessage
			json.Unmarshal(m.Payload, &chatMsg)
			return chatMsg.Sender == "ai"
		}
		return false
	})

	if !found {
		t.Fatal("never received AI agent response event")
	}

	var aiMsg ChatMessage
	json.Unmarshal(aiEvt.Payload, &aiMsg)
	if !strings.Contains(aiMsg.Content, "Mock AI response") {
		t.Errorf("unexpected AI message content: %s", aiMsg.Content)
	}
	if !strings.Contains(aiMsg.Content, "Current project") {
		t.Errorf("AI response did not include project context: %s", aiMsg.Content)
	}

	// 5. Verify Chat history
	sendMsg(t, conn, api.Message{
		ID:      "hist-1",
		Sender:  "user-1",
		Target:  "chat",
		Method:  "history",
		Payload: []byte(`{"channel":"test-channel"}`),
	})
	histResp := awaitResponse(t, collector, "hist-1-resp")
	var history []ChatMessage
	json.Unmarshal(histResp.Payload, &history)
	if len(history) < 2 {
		t.Errorf("expected at least 2 messages in history, got %d", len(history))
	}
}
