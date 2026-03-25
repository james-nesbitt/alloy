package tests

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func TestWorkspacePersistenceIntegration(t *testing.T) {
	manifest := map[string]any{
		"plugins": []map[string]any{},
	}

	// 1. Run first instance and register workspace
	cmd1, conn1, collector1, dataDir := setupTestCoreInsecure(t, "persist-1", manifest, true)

	ws := api.Workspace{
		ID:   "my-test-ws",
		Name: "Test Workspace",
		Path: "/tmp/test",
	}
	wsData, _ := json.Marshal(ws)

	sendMsg(t, conn1, api.Message{
		ID:      "reg-ws",
		Type:    api.TypeRequest,
		Sender:  "user",
		Target:  "kernel",
		Method:  "workspace:register",
		Payload: wsData,
	})
	awaitResponse(t, collector1, "reg-ws")

	// Set as active
	sendMsg(t, conn1, api.Message{
		ID:      "set-ws",
		Type:    api.TypeRequest,
		Sender:  "user",
		Target:  "kernel",
		Method:  "workspace:set_active",
		Payload: []byte(`"my-test-ws"`),
	})
	awaitResponse(t, collector1, "set-ws")

	conn1.Close()
	cmd1.Process.Kill()
	cmd1.Wait()
	time.Sleep(200 * time.Millisecond)

	// 2. Run second instance (restarting kernel with same data directory)
	// We manually construct the command to reuse the same dataDir
	cwd, _ := os.Getwd()
	corePath := filepath.Join(filepath.Dir(cwd), "build/dist/usr/libexec/alloy/alloy-core")
	socketPath2 := filepath.Join(dataDir, "alloy-2.sock")

	cmd2 := exec.Command(corePath,
		"--listen", "unix://"+socketPath2,
		"--data-dir", dataDir,
		"--insecure",
	)
	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr
	if err := cmd2.Start(); err != nil {
		t.Fatalf("failed to restart core: %v", err)
	}
	defer func() {
		cmd2.Process.Kill()
		cmd2.Wait()
	}()

	// Wait for socket
	var conn2 net.Conn
	var err error
	for i := 0; i < 20; i++ {
		conn2, err = net.Dial("unix", socketPath2)
		if err == nil {
			break
		}
		time.Sleep(time.Duration(100*(i+1)) * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("failed to connect to restarted core: %v", err)
	}
	defer conn2.Close()
	collector2 := NewMessageCollector(json.NewDecoder(conn2))

	sendMsg(t, conn2, api.Message{
		ID:     "get-ws",
		Type:   api.TypeRequest,
		Sender: "user",
		Target: "kernel",
		Method: "workspace:get_active",
	})

	resp := awaitResponse(t, collector2, "get-ws")
	var result api.Workspace
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v, payload: %s", err, string(resp.Payload))
	}

	if result.ID != "my-test-ws" {
		t.Errorf("expected workspace ID 'my-test-ws', got '%s'", result.ID)
	}
}
