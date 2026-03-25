package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func TestIAMEnforcement(t *testing.T) {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins")

	// Manifest with IAM and a target plugin
	manifest := map[string]any{
		"plugins": []map[string]any{
			{
				"id":        "test-health",
				"type":      "wasm",
				"path":      filepath.Join(buildDir, "health.wasm"),
				"load_time": "boot",
			},
		},
	}

	_, conn, collector, home := setupTestCore(t, "iam-test", manifest)
	defer os.RemoveAll(home)
	defer conn.Close()

	waitForPlugins(t, conn, collector, []string{"test-health"}, 30*time.Second)

	// 1. First, set an identity that has NO permissions
	// (Note: We use the 'admin' bypass or assume the current connection is 'user')
	// By default 'user' becomes 'guest' role which I gave many permissions to.

	// Let's create a new role 'restricted' with NO permissions.
	setPolicyReq, _ := json.Marshal(map[string]any{
		"policy": map[string]any{
			"role":        "restricted",
			"permissions": []string{},
		},
	})

	// We need to be authorized to SET a policy.
	// We'll use 'admin-sim' as sender. 
	
	sendMsg(t, conn, api.Message{
		ID:      "set-policy",
		Sender:  "admin-sim",
		Target:  "iam",
		Method:  "policy:set",
		Payload: setPolicyReq,
	})
	awaitResponse(t, collector, "set-policy")

	// Associate 'villain' actor with 'restricted' role
	setIdentReq, _ := json.Marshal(map[string]any{
		"actor": "villain",
		"role":  "restricted",
	})
	sendMsg(t, conn, api.Message{
		ID:      "set-identity",
		Sender:  "admin-sim",
		Target:  "iam",
		Method:  "identity:set",
		Payload: setIdentReq,
	})
	awaitResponse(t, collector, "set-identity")

	// 2. NOW, try to call health:status as 'villain'
	// In insecure mode, we can spoof the sender/actor.
	sendMsg(t, conn, api.Message{
		ID:     "attack-1",
		Sender: "villain", // The IPC server will set Actor = villain
		Target: "test-health",
		Method: "status",
	})

	// We expect a DENIAL from the system
	resp, ok := collector.AwaitID("attack-1", 5*time.Second)
	if !ok {
		t.Fatal("never received response for attack-1")
	}

	var body map[string]any
	json.Unmarshal(resp.Payload, &body)
	if errStr, ok := body["error"].(string); ok && strings.Contains(errStr, "denied") {
		t.Logf("SUCCESS: Access denied as expected: %s", errStr)
	} else {
		t.Errorf("FAIL: Access was NOT denied! Response: %s", string(resp.Payload))
	}
}
