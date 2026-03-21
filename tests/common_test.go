package tests

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

// StartCore starts the alloy-core as a subprocess and ensures it's killed when the test finishes.
func StartCore(t *testing.T, corePath string, args []string) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(corePath, args...)
	// On Linux, we can ensure the child dies if the parent (the test) dies.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}

	// For simple tests, let's keep the output as it might be useful for debugging
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start core: %v", err)
	}

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	return cmd
}

type MessageCollector struct {
	mu       sync.Mutex
	messages []api.Message
	decoder  *json.Decoder
}

func NewMessageCollector(decoder *json.Decoder) *MessageCollector {
	mc := &MessageCollector{
		decoder:  decoder,
		messages: make([]api.Message, 0),
	}
	go mc.run()
	return mc
}

func (mc *MessageCollector) run() {
	for {
		var msg api.Message
		if err := mc.decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				return
			}
			continue
		}
		mc.mu.Lock()
		mc.messages = append(mc.messages, msg)
		mc.mu.Unlock()
	}
}

func (mc *MessageCollector) Await(timeout time.Duration, filter func(api.Message) bool) (api.Message, bool) {
	deadline := time.Now().Add(timeout)
	lastIdx := 0
	for time.Now().Before(deadline) {
		mc.mu.Lock()
		for i := lastIdx; i < len(mc.messages); i++ {
			if filter(mc.messages[i]) {
				msg := mc.messages[i]
				mc.mu.Unlock()
				return msg, true
			}
		}
		lastIdx = len(mc.messages)
		mc.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	return api.Message{}, false
}

func (mc *MessageCollector) AwaitID(id string, timeout time.Duration) (api.Message, bool) {
	return mc.Await(timeout, func(m api.Message) bool {
		return m.ID == id || m.ID == id+"-resp"
	})
}

// setupTestCore starts the alloy-core with the given manifest and returns
// the process, connection, and a collector.
func setupTestCore(t *testing.T, label string, manifest map[string]any) (*exec.Cmd, net.Conn, *MessageCollector, string) {
	homeDir, _ := os.MkdirTemp("", "alloy-core-"+label+"-*")
	socketPath := filepath.Join(homeDir, "alloy.sock")

	cwd, _ := os.Getwd()
	corePath := filepath.Join(filepath.Dir(cwd), "build/core")

	manifestData, _ := json.Marshal(manifest)
	provisionPath := filepath.Join(homeDir, "provision.json")
	os.WriteFile(provisionPath, manifestData, 0644)

	cmd := StartCore(t, corePath, []string{
		"--socket", "unix://" + socketPath,
		"--home", homeDir,
		"--insecure",
		"--debug",
		"--provision", provisionPath,
	})
	
	// Wait for socket
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to core: %v", err)
	}

	return cmd, conn, NewMessageCollector(json.NewDecoder(conn)), homeDir
}

func waitForPlugins(t *testing.T, conn net.Conn, collector *MessageCollector, expectedIDs []string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	expected := make(map[string]bool)
	for _, id := range expectedIDs {
		expected[id] = true
	}

	for time.Now().Before(deadline) {
		id := "poll-discover-" + time.Now().Format("150405.000000")
		sendMsg(t, conn, api.Message{
			ID:     id,
			Type:   api.TypeRequest,
			Sender: "test-waiter",
			Target: "plugin-command-manager",
			Method: "discover",
		})

		resp, ok := collector.AwaitID(id, 5*time.Second)
		if !ok {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		var result struct {
			Targets []struct {
				ID string `json:"id"`
			} `json:"targets"`
		}
		json.Unmarshal(resp.Payload, &result)
		count := 0
		foundIDs := []string{}
		foundMap := make(map[string]bool)
		for _, target := range result.Targets {
			foundIDs = append(foundIDs, target.ID)
			if expected[target.ID] {
				foundMap[target.ID] = true
			}
		}
		count = len(foundMap)

		if count >= len(expected) {
			return
		}
		t.Logf("Waiting for plugins: %d/%d registered. Found in registry: %v", count, len(expected), foundIDs)
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for plugins: %v", expectedIDs)
}

// targetPlugin is a utility mock for testing routing and interception.
type targetPlugin struct {
	received chan api.Message
}

func (t *targetPlugin) ID() string                      { return "target-plugin" }
func (t *targetPlugin) Capabilities() []api.Capability { return nil }
func (t *targetPlugin) Shutdown(ctx context.Context) error { return nil }
func (t *targetPlugin) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	t.received <- msg
	return api.Message{}, nil
}

// sendMsg is a helper to encode and send a message over a network connection in tests.
func sendMsg(t *testing.T, conn net.Conn, msg api.Message) {
	t.Helper()
	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		t.Fatalf("failed to send message: %v", err)
	}
}

// awaitResponse waits for a message with a specific ID from the collector.
func awaitResponse(t *testing.T, collector *MessageCollector, id string) api.Message {
	t.Helper()
	msg, ok := collector.AwaitID(id, 5*time.Second)
	if !ok {
		t.Fatalf("timed out waiting for response ID %s", id)
	}
	return msg
}
