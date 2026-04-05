//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
)

var plugin *Plugin

func main() {
	plugin = NewPlugin("ai")
	plugin.Log(LogLevelInfo, "AI main starting")
	
	plugin.WithMetadata(
			"AI Agent",
			"Provides AI capabilities",
			"0.1.0",
			"Alloy Team",
		).
		WithCapability("ai:query", "Query the AI")

	plugin.Log(LogLevelInfo, "AI Serve() about to be called")
	plugin.Serve()
}
