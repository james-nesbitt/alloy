package tests

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func TestDynamicProvisioning(t *testing.T) {
	// 1. Setup minimal environment
	homeDir, _ := os.MkdirTemp("", "alloy-provision-test-*")
	defer os.RemoveAll(homeDir)

	socketPath := filepath.Join(homeDir, "alloy.sock")

	// Create a provision manifest
	provisionPath := filepath.Join(homeDir, "provision.json")
	manifest := map[string]any{
		"plugins": []map[string]any{
			{"id": "events", "type": "native"},
			{"id": "command-manager", "type": "native"},
			{"id": "kv", "type": "native"},
		},
	}
	manifestData, _ := json.Marshal(manifest)
	os.WriteFile(provisionPath, manifestData, 0644)

	// 2. Start alloy-core in minimal mode, but with the manifest
	// We'll run it as a subprocess or just use the packages.
	// For a true functional test, let's assume alloy-core is built.

	// Since we are in the same repo, we can just use the built binary from 'build/'
	corePath := "../build/dist/usr/libexec/alloy/alloy-core"
	if _, err := os.Stat(corePath); err != nil {
		t.Skip("alloy-core binary not found, skip functional test")
	}

	// Start core
	coreProcess := StartCore(t, corePath, []string{
		"--listen", "unix://" + socketPath,
		"--data-dir", homeDir,
		"--insecure",
		"--debug",
		"--provision", provisionPath,
	})
	coreProcess.Stdout = os.Stdout
	coreProcess.Stderr = os.Stderr

	// Wait for socket
	time.Sleep(2 * time.Second)

	// 3. Connect as a "frontend" and verify plugins were loaded
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to core: %v", err)
	}
	defer conn.Close()

	// 4. Send "discover" request to Command Manager
	discoverMsg := api.Message{
		ID:     "disc-1",
		Type:   api.TypeRequest,
		Sender: "test-frontend",
		Target: "command-manager",
		Method: "discover",
	}

	encoded, _ := json.Marshal(discoverMsg)
	conn.Write(append(encoded, '\n'))

	// 5. Read response
	decoder := json.NewDecoder(conn)
	var resp api.Message
	if err := decoder.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	t.Logf("Discover response: %s", string(resp.Payload))

	var result struct {
		Targets []map[string]any `json:"targets"`
	}
	json.Unmarshal(resp.Payload, &result)

	foundEvents := false
	foundKV := false
	for _, p := range result.Targets {
		if p["id"] == "events" {
			foundEvents = true
		}
		if p["id"] == "kv" {
			foundKV = true
		}
	}

	if !foundEvents || !foundKV {
		t.Errorf("expected plugins not found. events: %v, kv: %v", foundEvents, foundKV)
	}
}
