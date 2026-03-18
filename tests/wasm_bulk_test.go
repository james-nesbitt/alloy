package tests

import (
	"encoding/json"
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
			{"id": "plugin-command-manager", "type": "native"},
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

	// Wait for socket to appear
	for i := 0; i < 20; i++ {
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
		"plugin-secrets":  true,
		"plugin-health":   true,
	}

	t.Log("Polling for WASM plugins to register...")
	deadline := time.Now().Add(600 * time.Second)
	foundCount := 0
	for time.Now().Before(deadline) {
		sendMsg(t, conn, api.Message{
			ID:     "poll-discover",
			Type:   api.TypeRequest,
			Sender: "user",
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
			for _, target := range result.Targets {
				if pluginsToWait[target.ID] {
					count++
				}
			}
			if count >= len(pluginsToWait) {
				t.Log("All WASM plugins registered")
				foundCount = count
				break
			}
			t.Logf("Found %d/%d WASM plugins...", count, len(pluginsToWait))
		}
		time.Sleep(1 * time.Second)
	}

	if foundCount < len(pluginsToWait) {
		t.Fatal("Timed out waiting for WASM plugins")
	}

	// 1. Verify Health
	sendMsg(t, conn, api.Message{
		ID:     "health-1",
		Sender: "user",
		Target: "plugin-health",
		Method: "status",
	})
	awaitResponse(t, decoder, "health-1-resp")
	
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
