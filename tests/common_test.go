package tests

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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

// setupTestCore starts the alloy-core with the given manifest and returns
// the process, connection, and a decoder.
func setupTestCore(t *testing.T, label string, manifest map[string]any) (*exec.Cmd, net.Conn, *json.Decoder, string) {
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

// awaitResponse waits for a message with a specific ID from the decoder.
func awaitResponse(t *testing.T, decoder *json.Decoder, id string) api.Message {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var resp api.Message
		if err := decoder.Decode(&resp); err != nil {
			if err == io.EOF {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.ID == id || resp.ID == id+"-resp" {
			return resp
		}
	}
	t.Fatalf("timed out waiting for response ID %s", id)
	return api.Message{}
}
