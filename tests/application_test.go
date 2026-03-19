package tests

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
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
	// Setup environment
	homeDir, _ := os.MkdirTemp("", "alloy-app-test-*")
	defer os.RemoveAll(homeDir)

	socketPath := filepath.Join(homeDir, "alloy.sock")
	corePath := "../build/core"

	cwd, _ := os.Getwd()
	chatPath := filepath.Join(filepath.Dir(cwd), "build/wasm/chat.wasm")
	aiPath := filepath.Join(filepath.Dir(cwd), "build/wasm/ai.wasm")
	bufferPath := filepath.Join(filepath.Dir(cwd), "build/wasm/buffer.wasm")

	// Provisioning with necessary native plugins first
	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "plugin-events", "type": "native"},
			{"id": "plugin-command-manager", "type": "native"},
			{"id": "plugin-kv", "type": "native"},
			{"id": "plugin-logger", "type": "native"},
			{"id": "plugin-buffer-manager", "type": "wasm", "path": bufferPath},
			{"id": "plugin-chat", "type": "wasm", "path": chatPath},
			{"id": "plugin-ai-agent", "type": "wasm", "path": aiPath},
		},
	}
	manifestData, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(homeDir, "provision.json"), manifestData, 0644)

	// Start core
	provisionPath := filepath.Join(homeDir, "provision.json")
	coreProcess := StartCore(t, corePath, []string{
		"--socket", "unix://" + socketPath,
		"--home", homeDir,
		"--insecure",
		"--debug",
		"--provision", provisionPath,
	})
	coreProcess.Stdout = os.Stdout
	coreProcess.Stderr = os.Stderr

	// Wait for socket to appear
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Connect
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()
	decoder := json.NewDecoder(conn)

	// Polling for plugins to register...
	pluginsToWait := map[string]bool{
		"plugin-chat":     true,
		"plugin-ai-agent": true,
		"plugin-buffer-manager": true,
	}

	t.Log("Polling for WASM plugins to register...")
	deadline := time.Now().Add(600 * time.Second) // 10 minutes for slow compilation
	foundCount := 0
	for time.Now().Before(deadline) {
		sendMsg(t, conn, api.Message{
			ID:     "poll-discover",
			Type:   api.TypeRequest,
			Sender: "test-waiter",
			Target: "plugin-command-manager",
			Method: "discover",
		})

		var resp api.Message
		err := decoder.Decode(&resp)
		if err != nil {
			t.Logf("Decode error during poll: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if resp.ID == "poll-discover-resp" {
			var result struct {
				Targets []struct {
					ID string `json:"id"`
				} `json:"targets"`
			}
			json.Unmarshal(resp.Payload, &result)
			count := 0
			var foundIDs []string
			for _, target := range result.Targets {
				foundIDs = append(foundIDs, target.ID)
				if pluginsToWait[target.ID] {
					count++
				}
			}
			t.Logf("Found %d/%d WASM plugins. All targets: %v", count, len(pluginsToWait), foundIDs)
			if count >= len(pluginsToWait) {
				t.Log("All WASM plugins registered")
				foundCount = count
				break
			}
		} else {
			// Skip other messages like registered events
			t.Logf("Skipping message: %s %s", resp.ID, resp.Method)
		}
		time.Sleep(1 * time.Second)
	}

	if foundCount < len(pluginsToWait) {
		t.Fatal("Timed out waiting for WASM plugins")
	}

	// 1. Subscribe AI agent to chat events
	subReq, _ := json.Marshal(map[string]string{"topic": "chat:message"})
	sendMsg(t, conn, api.Message{
		ID:      "sub-1",
		Type:    api.TypeRequest,
		Sender:  "plugin-ai-agent",
		Target:  "plugin-events",
		Method:  "subscribe",
		Payload: subReq,
	})
	
	// Wait for subscription to be processed
	time.Sleep(100 * time.Millisecond)
	
	// 2. Send chat message from a user
	chatReq, _ := json.Marshal(map[string]string{
		"channel": "general",
		"content": "AI: hello test",
	})
	sendMsg(t, conn, api.Message{
		ID:      "chat-1",
		Type:    api.TypeRequest,
		Sender:  "user-1",
		Target:  "plugin-chat",
		Method:  "send",
		Payload: chatReq,
	})

	// Wait for AI to respond
	time.Sleep(2 * time.Second)

	// 3. Check history
	histReq, _ := json.Marshal(map[string]string{"channel": "general"})
	sendMsg(t, conn, api.Message{
		ID:      "hist-1",
		Type:    api.TypeRequest,
		Sender:  "user-1",
		Target:  "plugin-chat",
		Method:  "history",
		Payload: histReq,
	})

	var history []ChatMessage
	foundHist := false
	for i := 0; i < 20; i++ {
		var resp api.Message
		if err := decoder.Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.ID == "hist-1-resp" {
			json.Unmarshal(resp.Payload, &history)
			foundHist = true
			break
		}
	}

	if !foundHist {
		t.Fatal("never received hist-1-resp")
	}

	if len(history) < 2 {
		t.Errorf("expected at least 2 messages in history (user + AI), got %d", len(history))
	} else {
		foundAI := false
		for _, m := range history {
			if m.Sender == "plugin-ai-agent" {
				foundAI = true
				break
			}
		}
		if !foundAI {
			t.Error("AI agent response not found in chat history")
		}
	}
}
