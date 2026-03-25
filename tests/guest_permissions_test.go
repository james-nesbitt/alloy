package tests

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func TestGuestPermissions(t *testing.T) {
	// Standard setup in SECURE mode
	_, conn, collector, home := setupTestCoreSecure(t, "guest-perms", nil)
	defer os.RemoveAll(home)
	defer conn.Close()

	// 1. Should be allowed to call discovery as guest
	sendMsg(t, conn, api.Message{
		ID:     "guest-discover",
		Sender: "guest-user",
		Target: "command-manager",
		Method: "discover",
	})

	resp, ok := collector.AwaitID("guest-discover", 5*time.Second)
	if !ok {
		t.Fatal("never received response for guest-discover")
	}
	t.Logf("Guest discovery response: %s", string(resp.Payload))
	if strings.Contains(string(resp.Payload), "denied") {
		t.Errorf("FAIL: Guest should be able to DISCOVER core services!")
	}

	// 2. Should be DENIED from setting a policy
	setPolicyReq, _ := json.Marshal(map[string]any{
		"policy": map[string]any{
			"role":        "hacker",
			"permissions": []string{"*"},
		},
	})
	sendMsg(t, conn, api.Message{
		ID:      "guest-attack",
		Sender:  "guest-user",
		Target:  "iam",
		Method:  "policy:set",
		Payload: setPolicyReq,
	})

	resp, ok = collector.AwaitID("guest-attack", 5*time.Second)
	if !ok {
		t.Fatal("never received response for guest-attack")
	}
	t.Logf("Guest attack response: %s", string(resp.Payload))
	if !strings.Contains(string(resp.Payload), "denied") {
		t.Errorf("FAIL: Guest should NOT be able to SET IAM policies!")
	}
}
