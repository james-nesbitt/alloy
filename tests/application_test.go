package tests

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/plugins/native"
)

func TestApplicationPlugins(t *testing.T) {
	// Setup environment
	homeDir, _ := os.MkdirTemp("", "alloy-app-test-*")
	defer os.RemoveAll(homeDir)

	socketPath := filepath.Join(homeDir, "alloy.sock")
	corePath := "../bin/alloy-core"

	// Provisioning with new application plugins
	manifest := map[string]any{
		"plugins": []map[string]string{
			{"id": "plugin-events", "type": "native"},
			{"id": "plugin-kv", "type": "native"},
			{"id": "plugin-buffer-manager", "type": "native"},
			{"id": "plugin-chat", "type": "native"},
			{"id": "plugin-ai-agent", "type": "native"},
		},
	}
	manifestData, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(homeDir, "provision.json"), manifestData, 0644)

	// Start core
	coreProcess := exec.Command(corePath, "--socket", "unix://"+socketPath, "--home", homeDir, "--insecure", "--debug")
	// coreProcess.Stdout = os.Stdout
	// coreProcess.Stderr = os.Stderr
	if err := coreProcess.Start(); err != nil {
		t.Fatalf("failed to start core: %v", err)
	}
	defer coreProcess.Process.Kill()

	time.Sleep(2 * time.Second)

	// Connect
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()
	decoder := json.NewDecoder(conn)

	// 1. Subscribe AI agent to chat events
	// We'll simulate the system or an admin doing this.
	subReq, _ := json.Marshal(map[string]string{"topic": "chat:message"})
	sendMsg(t, conn, api.Message{
		ID:      "sub-1",
		Type:    api.TypeRequest,
		Sender:  "plugin-ai-agent",
		Target:  "plugin-events",
		Method:  "subscribe",
		Payload: subReq,
	})
	
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

	// Wait for AI to respond via the event loop
	// We should see a "chat:message" event or the AI's response message in history
	time.Sleep(1 * time.Second)

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

	var history []native.ChatMessage
	foundHist := false
	for i := 0; i < 10; i++ {
		var resp api.Message
		if err := decoder.Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		t.Logf("Received response for: %s", resp.ID)
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
		for i, m := range history {
			t.Logf("Msg %d: from=%s content=%s", i, m.Sender, m.Content)
		}
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

func sendMsg(t *testing.T, conn net.Conn, msg api.Message) {
	data, _ := json.Marshal(msg)
	if _, err := conn.Write(append(data, '\n')); err != nil {
		t.Fatalf("failed to send message %s: %v", msg.ID, err)
	}
}
