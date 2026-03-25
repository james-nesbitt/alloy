package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func TestHotReloading(t *testing.T) {
	t.Skip("Hot reloading feature needs to be updated for Core vs WASM architecture")
	// 1. Setup core with wasm-manager and health
	cwd, _ := os.Getwd()
	healthPath := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins/health.wasm")

	// Ensure it exists
	if _, err := os.Stat(healthPath); err != nil {
		t.Skip("build/dist/usr/lib/alloy/plugins/health.wasm not found, run just build-all first")
	}

	manifest := map[string]any{
		"plugins": []map[string]any{},
	}

	cmd, conn, collector, _ := setupTestCore(t, "hot-reload", manifest)
	defer cmd.Process.Kill()
	defer conn.Close()

	// 2. Load the health plugin
	loadID := "load-1"
	loadPayload, _ := json.Marshal(map[string]any{
		"id":   "health-wasm",
		"path": healthPath,
	})
	sendMsg(t, conn, api.Message{
		ID:      loadID,
		Type:    api.TypeRequest,
		Sender:  "test-loader",
		Target:  "wasm-manager",
		Method:  "load",
		Payload: loadPayload,
	})

	// Wait for registration
	waitForPlugins(t, conn, collector, []string{"health-wasm"}, 10*time.Second)

	// 3. Verify health works
	pingID := "ping-v1"
	sendMsg(t, conn, api.Message{
		ID:     pingID,
		Type:   api.TypeRequest,
		Sender: "test-pinger",
		Target: "health-wasm",
		Method: "status",
	})
	resp1 := awaitResponse(t, collector, pingID)
	var res1 struct {
		Source string `json:"source"`
	}
	json.Unmarshal(resp1.Payload, &res1)
	if res1.Source == "" {
		t.Fatalf("Response source is empty. Payload: %s", string(resp1.Payload))
	}

	// 4. Enable Watch
	watchID := "watch-1"
	watchPayload, _ := json.Marshal(map[string]any{"id": "health-wasm"})
	sendMsg(t, conn, api.Message{
		ID:      watchID,
		Type:    api.TypeRequest,
		Sender:  "test-watcher",
		Target:  "wasm-manager",
		Method:  "watch",
		Payload: watchPayload,
	})
	awaitResponse(t, collector, watchID)

	// 5. Simulate update by 'touching' the file
	now := time.Now()
	if err := os.Chtimes(healthPath, now, now); err != nil {
		t.Fatalf("Failed to touch wasm file: %v", err)
	}

	// We need to wait for FSNotify event to trigger reload
	time.Sleep(2 * time.Second)

	// 6. Verify it's still alive and responded (meaning it reloaded successfully)
	ping2ID := "ping-v2"
	sendMsg(t, conn, api.Message{
		ID:     ping2ID,
		Type:   api.TypeRequest,
		Sender: "test-pinger",
		Target: "health-wasm",
		Method: "status",
	})
	resp2 := awaitResponse(t, collector, ping2ID)
	var res2 struct {
		Source string `json:"source"`
	}
	json.Unmarshal(resp2.Payload, &res2)
	if res2.Source == "" {
		t.Fatalf("Response source is empty after reload")
	}
}
