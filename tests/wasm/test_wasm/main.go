//go:build wasip1 || wasm

package main

import (
	"encoding/json"
	"github.com/james-nesbitt/alloy/build/gen/bindings/guest"
)

func main() {
	// Initialize the plugin
	pluginID := "test-wasm"
	caps := []guest.AlloyCapability{
		{
			Method:      "test:hello",
			Description: "Responds with a hello message",
		},
		{
			Method:      "test:echo",
			Description: "Echoes back the input",
		},
	}

	// Initialize the plugin with the host
	guest.AlloyInit(pluginID, caps)

	// Signal that the plugin is ready
	guest.AlloyStarted()

	// Test KV storage
	guest.AlloyLog("info", "Testing KV storage...")
	if !guest.AlloyKvSet("test-key", []byte("test-value")) {
		guest.AlloyLog("error", "Failed to set KV value")
	}

	value := guest.AlloyKvGet("test-key")
	if !value.Set || string(value.Value) != "test-value" {
		guest.AlloyLog("error", "Failed to get KV value")
	} else {
		guest.AlloyLog("info", "KV storage test passed")
	}

	// Keep the plugin running
	for {
		// Get the next message
		msgOpt := guest.AlloyGetNextMessage()
		if !msgOpt.Set {
			guest.AlloyYield()
			continue
		}

		msg := msgOpt.Value
		guest.AlloyLog("info", "Received message: "+msg.Method)

		// Handle the message
		var resp guest.AlloyMessage
		if msg.Method == "test:hello" {
			resp = guest.AlloyMessage{
				Id:      msg.Id + "-response",
				Method:  msg.Method,
				Sender:  pluginID,
				Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
				Payload: []byte(`{"message":"Hello from WASM!"}`),
			}
		} else if msg.Method == "test:echo" {
			resp = guest.AlloyMessage{
				Id:      msg.Id + "-response",
				Method:  msg.Method,
				Sender:  pluginID,
				Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
				Payload: msg.Payload, // Echo back the payload
			}
		} else {
			errMsg := map[string]string{"error": "method_not_found"}
			errData, _ := json.Marshal(errMsg)
			resp = guest.AlloyMessage{
				Id:      msg.Id + "-response",
				Method:  msg.Method,
				Sender:  pluginID,
				Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
				Payload: errData,
			}
		}

		// Send the response
		guest.AlloySendResponse(resp)
	}
}