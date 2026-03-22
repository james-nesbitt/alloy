//go:build wasip1 || wasm

package main

import (
	"encoding/json"
	"github.com/jnesbitt/alloy-go/pkg/wasm/guest"
)

func main() {
	// Create a new plugin
	plugin := guest.NewPlugin("test-plugin")

	// Add capabilities
	plugin.WithCapability("test:hello", "Responds with a hello message")
	plugin.WithCapability("test:echo", "Echoes back the input")

	// Set up message handlers
	plugin.Handle("test:hello", func(msg guest.AlloyMessage) guest.AlloyMessage {
		plugin.Log("info", "Handling test:hello message")
		return guest.Reply(msg, map[string]string{"message": "Hello from WIT plugin!"})
	})

	plugin.Handle("test:echo", func(msg guest.AlloyMessage) guest.AlloyMessage {
		plugin.Log("info", "Handling test:echo message")
		var payload map[string]string
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			if text, ok := payload["text"]; ok {
				return guest.Reply(msg, map[string]string{"echo": text})
			}
		}
		return guest.ErrorReply(msg, "invalid_payload")
	})

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "Test plugin initializing")
		return nil
	})

	// Set up background process
	plugin.OnStart(func() {
		plugin.Log("info", "Test plugin background process started")
	})

	// Run the plugin
	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}