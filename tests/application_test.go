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

type ChatMessage struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

func TestApplicationPlugins(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/wasm")

	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "plugin-events", "type": "native"},
			{"id": "plugin-command-manager", "type": "native"},
			{"id": "plugin-kv", "type": "native"},
			{"id": "plugin-logger", "type": "native"},
			{"id": "plugin-buffer-manager", "type": "wasm", "path": filepath.Join(buildDir, "buffer.wasm")},
			{"id": "plugin-chat", "type": "wasm", "path": filepath.Join(buildDir, "chat.wasm")},
			{"id": "plugin-ai-agent", "type": "wasm", "path": filepath.Join(buildDir, "ai.wasm"), "memory_limit_mb": 64},
		},
	}

	_, conn, collector, home := setupTestCore(t, "app-test", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	t.Log("Polling for WASM plugins to register...")
	expected := []string{
		"plugin-chat", "plugin-ai-agent", "plugin-buffer-manager",
	}
	waitForPlugins(t, conn, collector, expected, 30*time.Second)
	t.Log("All WASM plugins registered")

	// 1. Subscribe to chat events for the test connection and the AI agent
	subReq, _ := json.Marshal(map[string]string{"topic": "chat:message"})
	// Test connection subscription
	sendMsg(t, conn, api.Message{
		ID:      "sub-test",
		Sender:  "user-1",
		Target:  "plugin-events",
		Method:  "subscribe",
		Payload: subReq,
	})
	awaitResponse(t, collector, "sub-test-resp")

	// AI Agent subscription
	sendMsg(t, conn, api.Message{
		ID:      "sub-ai",
		Sender:  "plugin-ai-agent",
		Target:  "plugin-events",
		Method:  "subscribe",
		Payload: subReq,
	})
	// We don't await the response here because it goes to the plugin, not us
	time.Sleep(200 * time.Millisecond) // Give time for sub to process

	// 2. Clear old collector messages before test
	time.Sleep(100 * time.Millisecond)

	// 3. User sends a message that should trigger AI response
	chatReq, _ := json.Marshal(map[string]string{
		"channel": "test-channel",
		"content": "AI: hello world",
	})
	sendMsg(t, conn, api.Message{
		ID:      "chat-1",
		Sender:  "user-1",
		Target:  "plugin-chat",
		Method:  "send",
		Payload: chatReq,
	})
	awaitResponse(t, collector, "chat-1-resp")

	// 4. Verify AI Response via event bus
	aiEvt, found := collector.Await(10*time.Second, func(m api.Message) bool {
		if m.Method == "chat:message" && m.Type == "event" {
			var chatMsg ChatMessage
			json.Unmarshal(m.Payload, &chatMsg)
			return chatMsg.Sender == "plugin-ai-agent"
		}
		return false
	})

	if !found {
		t.Fatal("never received AI agent response event")
	}
	
	var aiMsg ChatMessage
	json.Unmarshal(aiEvt.Payload, &aiMsg)
	if !strings.Contains(aiMsg.Content, "I processed your request") {
		t.Errorf("unexpected AI message content: %s", aiMsg.Content)
	}

	// 5. Verify Chat history
	sendMsg(t, conn, api.Message{
		ID:      "hist-1",
		Sender:  "user-1",
		Target:  "plugin-chat",
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
