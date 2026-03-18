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

// setupTestCore starts the alloy-core with the given manifest and returns
// the process, connection, and a decoder.
func setupTestCore(t *testing.T, label string, manifest map[string]any) (*exec.Cmd, net.Conn, *json.Decoder, string) {
	homeDir, _ := os.MkdirTemp("", "alloy-wasm-"+label+"-*")
	socketPath := filepath.Join(homeDir, "alloy.sock")
	
	cwd, _ := os.Getwd()
	corePath := filepath.Join(filepath.Dir(cwd), "build/core")

	manifestData, _ := json.Marshal(manifest)
	provisionPath := filepath.Join(homeDir, "provision.json")
	os.WriteFile(provisionPath, manifestData, 0644)

	cmd := exec.Command(corePath, 
		"--socket", "unix://"+socketPath, 
		"--home", homeDir, 
		"--insecure", 
		"--debug", 
		"--provision", provisionPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start core: %v", err)
	}

	// Wait for socket
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		cmd.Process.Kill()
		t.Fatalf("failed to connect to core: %v", err)
	}

	return cmd, conn, json.NewDecoder(conn), homeDir
}

func waitForPlugins(t *testing.T, conn net.Conn, decoder *json.Decoder, expectedIDs []string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	expected := make(map[string]bool)
	for _, id := range expectedIDs {
		expected[id] = true
	}

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
			time.Sleep(500 * time.Millisecond)
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
				if expected[target.ID] {
					count++
				}
			}
			if count >= len(expected) {
				return
			}
			t.Logf("Waiting for plugins: %d/%d registered...", count, len(expected))
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("timed out waiting for plugins: %v", expectedIDs)
}

func TestWasmLoadMock(t *testing.T) {
	cwd, _ := os.Getwd()
	mockPath := filepath.Join(filepath.Dir(cwd), "build/wasm/mock.wasm")
	if _, err := os.Stat(mockPath); os.IsNotExist(err) {
		t.Skip("mock.wasm not found")
	}

	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "plugin-events", "type": "native"},
			{"id": "plugin-command-manager", "type": "native"},
			{"id": "plugin-mock", "type": "wasm", "path": mockPath},
		},
	}

	cmd, conn, decoder, home := setupTestCore(t, "mock", manifest)
	defer os.RemoveAll(home)
	defer cmd.Process.Kill()
	defer conn.Close()

	waitForPlugins(t, conn, decoder, []string{"plugin-mock"}, 30*time.Second)
}

func TestWasmLoadBulk(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/wasm")
	
	plugins := []string{"ai", "secrets", "health", "chat", "buffer"}
	wasmPlugins := []map[string]any{
		{"id": "plugin-events", "type": "native"},
		{"id": "plugin-command-manager", "type": "native"},
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
			})
			expectedIDs = append(expectedIDs, id)
		}
	}

	manifest := map[string]any{"plugins": wasmPlugins}
	cmd, conn, decoder, home := setupTestCore(t, "bulk", manifest)
	defer os.RemoveAll(home)
	defer cmd.Process.Kill()
	defer conn.Close()

	waitForPlugins(t, conn, decoder, expectedIDs, 300*time.Second)
}

func TestWasmFunctionalSuite(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/wasm")

	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "plugin-events", "type": "native"},
			{"id": "plugin-command-manager", "type": "native"},
			{"id": "plugin-kv", "type": "native"},
			{"id": "plugin-chat", "type": "wasm", "path": filepath.Join(buildDir, "chat.wasm")},
			{"id": "plugin-ai-agent", "type": "wasm", "path": filepath.Join(buildDir, "ai.wasm")},
			{"id": "plugin-secrets", "type": "wasm", "path": filepath.Join(buildDir, "secrets.wasm")},
			{"id": "plugin-health", "type": "wasm", "path": filepath.Join(buildDir, "health.wasm")},
			{"id": "plugin-buffer-manager", "type": "wasm", "path": filepath.Join(buildDir, "buffer.wasm")},
		},
	}

	cmd, conn, decoder, home := setupTestCore(t, "full", manifest)
	defer os.RemoveAll(home)
	defer cmd.Process.Kill()
	defer conn.Close()

	expected := []string{
		"plugin-chat", "plugin-ai-agent", "plugin-secrets", 
		"plugin-health", "plugin-buffer-manager",
	}
	waitForPlugins(t, conn, decoder, expected, 300*time.Second)

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
		Method:  "open",
		Payload: openReq,
	})
	awaitResponse(t, decoder, "buf-wasm-1-resp")

	// 4. Verify Chat and AI Agent (Subscription and Reaction)
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
	for i := 0; i < 100; i++ {
		var chatResp api.Message
		if err := decoder.Decode(&chatResp); err != nil {
			t.Fatalf("decode err at index %d: %v", i, err)
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
