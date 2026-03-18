package tests

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

func TestWasmBulkMigration(t *testing.T) {
	// Setup environment
	homeDir, _ := os.MkdirTemp("", "alloy-wasm-bulk-*")
	defer os.RemoveAll(homeDir)

	socketPath := filepath.Join(homeDir, "alloy.sock")
	corePath := "../build/core"

	cwd, _ := os.Getwd()
	aiPath := filepath.Join(cwd, "../build/wasm/ai.wasm")
	secretsPath := filepath.Join(cwd, "../build/wasm/secrets.wasm")
	healthPath := filepath.Join(cwd, "../build/wasm/health.wasm")
	chatPath := filepath.Join(cwd, "../build/wasm/chat.wasm")

	// Create provision.json
	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "plugin-events", "type": "native"},
			{"id": "plugin-kv", "type": "native"},
			{"id": "plugin-chat", "type": "wasm", "path": chatPath},
			{"id": "plugin-ai-agent", "type": "wasm", "path": aiPath},
			{"id": "plugin-secrets", "type": "wasm", "path": secretsPath},
			{"id": "plugin-health", "type": "wasm", "path": healthPath},
		},
	}
	manifestData, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(homeDir, "provision.json"), manifestData, 0644)

	// Start core
	coreProcess := exec.Command(corePath, "--socket", "unix://"+socketPath, "--home", homeDir, "--insecure", "--debug")
	coreProcess.Stdout = os.Stdout
	coreProcess.Stderr = os.Stderr
	if err := coreProcess.Start(); err != nil {
		t.Fatalf("failed to start core: %v", err)
	}
	defer coreProcess.Process.Kill()

	time.Sleep(5 * time.Second)

	// Connect
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()
	decoder := json.NewDecoder(conn)

	// 1. Verify Health (with retry since provision might still be in progress)
	var healthResp api.Message
	for i := 0; i < 5; i++ {
		sendMsg(t, conn, api.Message{
			ID:     fmt.Sprintf("health-%d", i),
			Sender: "user",
			Target: "plugin-health",
			Method: "status",
		})
		
		var m api.Message
		err := decoder.Decode(&m)
		if err == nil && m.ID == fmt.Sprintf("health-%d-resp", i) {
			healthResp = m
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	
	if healthResp.ID == "" {
		t.Fatal("never received health response")
	}

	// 2. Verify Secrets
	storeReq, _ := json.Marshal(map[string]string{"id": "db_pass", "value": "password123"})
	sendMsg(t, conn, api.Message{
		ID:      "secret-1",
		Sender:  "user",
		Target:  "plugin-secrets",
		Method:  "store_secret",
		Payload: storeReq,
	})
	awaitResponse(t, decoder, "secret-1-resp")

	getReq, _ := json.Marshal(map[string]string{"id": "db_pass"})
	sendMsg(t, conn, api.Message{
		ID:      "secret-2",
		Sender:  "user",
		Target:  "plugin-secrets",
		Method:  "get_secret",
		Payload: getReq,
	})
	resp := awaitResponse(t, decoder, "secret-2-resp")
	if !strings.Contains(string(resp.Payload), "password123") {
		t.Errorf("secret mismatch, got: %s", string(resp.Payload))
	}

	// 3. Verify AI Agent (Subscription and Reaction)
	chatReq, _ := json.Marshal(map[string]string{
		"channel": "ai-test",
		"content": "AI: test bulk migration",
	})
	sendMsg(t, conn, api.Message{
		ID:      "chat-1",
		Sender:  "user",
		Target:  "plugin-chat",
		Method:  "send",
		Payload: chatReq,
	})

	// AI should react to the chat event
	foundAI := false
	for i := 0; i < 50; i++ {
		var chatResp api.Message
		if err := decoder.Decode(&chatResp); err != nil {
			t.Fatalf("decode err: %v", err)
		}
		if chatResp.Sender == "plugin-ai-agent" && chatResp.Method == "send" {
			foundAI = true
			t.Logf("AI Response found: %s", string(chatResp.Payload))
			break
		}
	}
	if !foundAI {
		t.Error("never received AI agent response via Route in WASM")
	}
}

func awaitResponse(t *testing.T, dec *json.Decoder, id string) api.Message {
	for i := 0; i < 100; i++ {
		var m api.Message
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("failed to decode while waiting for %s: %v", id, err)
		}
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("timed out waiting for message %s", id)
	return api.Message{}
}
