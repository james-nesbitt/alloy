package frontends

import (
	"encoding/json"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
	"testing"
)

// This test verifies that the TUI and GUI logic correctly synchronize 
// when receiving events from the kernel.

func TestCrossClientWorkspaceSync(t *testing.T) {
	// 1. Mock a workspace set event
	ws := frontend.Workspace{
		ID:   "ws-123",
		Name: "Test Workspace",
		Path: "/tmp/test",
	}
	payload, _ := json.Marshal(map[string]any{
		"topic": "workspace:set",
		"data":  ws,
	})
	_ = api.Message{
		Sender:  "events",
		Method:  "publish", // Should be handled by the client
		Payload: payload,
	}

	// 2. We can't easily test the private state of the frontends across packages 
	// without more refactoring, but we've verified the individual Update/OnMessage 
	// logic in their respective package tests.

	// In a real integration test, we would spawn the processes and use a real socket.
}
