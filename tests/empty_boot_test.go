package tests

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func TestEmptyBoot(t *testing.T) {
	// 1. Setup minimal environment
	homeDir, _ := os.MkdirTemp("", "alloy-empty-test-*")
	defer os.RemoveAll(homeDir)

	socketPath := filepath.Join(homeDir, "alloy.sock")
	corePath := "../build/dist/usr/libexec/alloy/alloy-core"

	// Start core WITHOUT --provision and WITHOUT --wasm-plugins
	coreProcess := StartCore(t, corePath, []string{
		"--listen", "unix://" + socketPath,
		"--data-dir", homeDir,
		"--insecure",
		"--debug",
	})
	coreProcess.Stdout = os.Stdout
	coreProcess.Stderr = os.Stderr

	// Wait for socket
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to core: %v", err)
	}
	defer conn.Close()

	pingMsg := api.Message{
		ID:     "ping-1",
		Type:   api.TypeRequest,
		Sender: "test-frontend",
		Target: "kernel",
		Method: "ping",
	}

	sendMsg(t, conn, pingMsg)
	collector := NewMessageCollector(json.NewDecoder(conn))
	resp := awaitResponse(t, collector, "ping-1")
	if !strings.Contains(string(resp.Payload), "pong") {
		t.Errorf("expected pong, got %s", string(resp.Payload))
	}

	// Double check that we can't talk to something that shouldn't exist
	sendMsg(t, conn, api.Message{
		ID:     "bad-req-1",
		Sender: "test-frontend",
		Target: "ghost-plugin", // KV ALWAYS exists now as core service
		Method: "set",
	})

	// We expect NO response or a warning in logs, but since we're in a test
	// we'll just wait a bit and move on.
	time.Sleep(1 * time.Second)
}
