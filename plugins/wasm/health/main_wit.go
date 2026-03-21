//go:build wasip1 || wasm

package main

import (
	"github.com/jnesbitt/alloy-go/pkg/wasm2/guest"
)

func main() {
	// Create a new WIT-based plugin
	plugin := guest.NewPlugin("health").
		WithMetadata(
			"Health Plugin", 
			"Provides health status information for the WASM instance",
			"0.1.0", 
			"Alloy Team",
		).
		WithTags("monitoring", "health", "status").
		WithCapability("status", "Get the health status of this WASM instance")

	// Set up message handler
	plugin.Handle("status", func(msg guest.AlloyMessage) guest.AlloyMessage {
		status := map[string]string{
			"status": "healthy",
			"uptime": "wasm-monitored",
			"source": "wasm-wit",
		}
		return guest.Reply(msg, status)
	})

	// Run the plugin
	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}