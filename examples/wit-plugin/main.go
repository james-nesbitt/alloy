//go:build tinygo || wasip1 || wasm

package main

import (
	"github.com/jnesbitt/alloy-go/pkg/wasm/bindings/guest"
)

func main() {
	// Initialize the plugin with WIT bindings
	plugin := NewExamplePlugin()
	plugin.Run()
}

// ExamplePlugin demonstrates how to use the WIT bindings
type ExamplePlugin struct {
	id           string
	capabilities []guest.AlloyCapability
}

// NewExamplePlugin creates a new example plugin
func NewExamplePlugin() *ExamplePlugin {
	return &ExamplePlugin{
		id: "example-plugin",
		capabilities: []guest.AlloyCapability{
			{
				Method:      "example:hello",
				Description: "Responds with a hello message",
			},
		},
	}
}

// Run starts the plugin with WIT bindings
func (p *ExamplePlugin) Run() {
	// Initialize the plugin with the host
	guest.AlloyInit(p.id, p.capabilities)

	// Signal that the plugin is ready
	guest.AlloyStarted()

	// In a real implementation, we would:
	// 1. Set up message handlers
	// 2. Start background processes
	// 3. Enter the main message loop

	// For now, just demonstrate the WIT interface
	guest.AlloyLog("info", "Example plugin started with WIT bindings")

	// Keep the plugin running
	select {}
}

// handleMessage demonstrates how message handling would work
func (p *ExamplePlugin) handleMessage(msg guest.AlloyMessage) guest.AlloyMessage {
	guest.AlloyLog("debug", "Received message: "+msg.Method)

	// Handle different message types
	if msg.Method == "example:hello" {
		return guest.AlloyMessage{
			Id:      msg.Id + "-response",
			Method:  msg.Method,
			Sender:  p.id,
			Target:  msg.Sender,
			Payload: []byte(`{"message":"Hello from WIT plugin!"}`),
		}
	}

	// Default response for unknown methods
	return guest.AlloyMessage{
		Id:      msg.Id + "-response",
		Method:  msg.Method,
		Sender:  p.id,
		Target:  msg.Sender,
		Payload: []byte(`{"error":"method_not_found"}`),
	}
}