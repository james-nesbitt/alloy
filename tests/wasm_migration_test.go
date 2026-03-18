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
)

func TestWasmMigration(t *testing.T) {
	// Setup environment
	homeDir, _ := os.MkdirTemp("", "alloy-wasm-test-*")
	defer os.RemoveAll(homeDir)

	socketPath := filepath.Join(homeDir, "alloy.sock")
	corePath := "../build/core"

	// Copy the built WASM plugins to a stable location or just absolute path
	cwd, _ := os.Getwd()
	chatPath := filepath.Join(cwd, "../build/wasm/chat.wasm")
	bufferPath := filepath.Join(cwd, "../build/wasm/buffer.wasm")

	// Create provision.json
	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "plugin-events", "type": "native"},
			{"id": "plugin-command-manager", "type": "native"},
			{"id": "plugin-kv", "type": "native"},
			{"id": "plugin-chat", "type": "wasm", "path": chatPath},
			{"id": "plugin-buffer-manager", "type": "wasm", "path": bufferPath},
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

	// Polling for plugins...
	pluginsToWait := map[string]bool{
		"plugin-chat":           true,
		"plugin-buffer-manager": true,
	}
	deadline := time.Now().Add(600 * time.Second)
	foundCount := 0
	for time.Now().Before(deadline) {
		sendMsg(t, conn, api.Message{
			ID:     "poll-discover",
			Type:   api.TypeRequest,
			Sender: "user-1",
			Target: "plugin-command-manager",
			Method: "discover",
		})

		var m api.Message
		if err := decoder.Decode(&m); err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		if m.ID == "poll-discover-resp" {
			var result struct {
				Targets []struct {
					ID string `json:"id"`
				} `json:"targets"`
			}
			json.Unmarshal(m.Payload, &result)
			count := 0
			for _, target := range result.Targets {
				if pluginsToWait[target.ID] {
					count++
				}
			}
			if count >= len(pluginsToWait) {
				foundCount = count
				break
			}
		}
		time.Sleep(1 * time.Second)
	}

	if foundCount < len(pluginsToWait) {
		t.Fatal("Timed out waiting for WASM plugins")
	}

	// 1. Test WASM Chat Plugin
	chatReq, _ := json.Marshal(map[string]string{
		"channel": "general",
		"content": "Hello from WASM!",
	})
	sendMsg(t, conn, api.Message{
		ID:      "chat-wasm-1",
		Type:    api.TypeRequest,
		Sender:  "user-1",
		Target:  "plugin-chat",
		Method:  "send",
		Payload: chatReq,
	})

	var resp api.Message
	// We might get other messages (registration events), loop until we get our response
	for i := 0; i < 20; i++ {
		if err := decoder.Decode(&resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.ID == "chat-wasm-1-resp" {
			break
		}
	}

	if resp.ID != "chat-wasm-1-resp" {
		t.Fatal("never received chat-wasm-1-resp")
	}
	t.Logf("Chat response: %s", string(resp.Payload))

	// 2. Test WASM Buffer Plugin
	openReq, _ := json.Marshal(map[string]string{
		"id":   "doc-1",
		"type": "text",
	})
	sendMsg(t, conn, api.Message{
		ID:      "buf-wasm-1",
		Type:    api.TypeRequest,
		Sender:  "user-1",
		Target:  "plugin-buffer-manager",
		Method:  "open",
		Payload: openReq,
	})

	for i := 0; i < 20; i++ {
		if err := decoder.Decode(&resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.ID == "buf-wasm-1-resp" {
			break
		}
	}
	if resp.ID != "buf-wasm-1-resp" {
		t.Fatal("never received buf-wasm-1-resp")
	}
	t.Logf("Buffer response: %s", string(resp.Payload))
}
