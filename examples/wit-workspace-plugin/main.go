//go:build wasip1 || wasm

package main

import (
	"encoding/json"
	"github.com/james-nesbitt/alloy/pkg/wasm/guest"
	alloy "github.com/james-nesbitt/alloy/build/gen/bindings/guest"
)

func main() {
	plugin := guest.NewPlugin("workspace-test")

	plugin.RegisterMethod("test", "Test workspace WIT calls", func(msg guest.Message) *guest.Message {
		// 1. Register a workspace
		ws := alloy.AlloyWorkspace{
			Id:   "test-ws",
			Name: "Test Workspace",
			Path: "/tmp/test",
			Metadata: []alloy.AlloyTuple2StringStringT{
				{F0: "env", F1: "dev"},
			},
		}
		plugin.RegisterWorkspace(ws)

		// 2. Verify active workspace
		active, ok := plugin.GetActiveWorkspace()
		if !ok || active.Id != "test-ws" {
			return plugin.ReplyError(msg, "Active workspace not found or incorrect")
		}

		// 3. List workspaces
		list := plugin.ListWorkspaces()
		if len(list) == 0 {
			return plugin.ReplyError(msg, "Workspace list empty")
		}

		// 4. Test meta
		foundMeta := false
		for _, m := range active.Metadata {
			if m.F0 == "env" && m.F1 == "dev" {
				foundMeta = true
				break
			}
		}
		if !foundMeta {
			return plugin.ReplyError(msg, "Metadata not found in active workspace")
		}

		plugin.Log(guest.LogLevelInfo, "Workspace WIT test passed")

		payload, _ := json.Marshal(map[string]any{
			"status": "passed",
			"active": active.Id,
			"count": len(list),
		})
		return &guest.Message{
			ID:      msg.ID + "-resp",
			Method:  msg.Method,
			Payload: payload,
			Target:  msg.Sender,
		}
	})

	plugin.Serve()
}
