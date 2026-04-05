//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
)

var plugin *Plugin

func main() {
	plugin = NewPlugin("repository").
		WithMetadata(
			"Repository Service",
			"Manages git repositories and version control integration",
			"0.1.0",
			"Alloy Team",
		).
		WithCapability("config:update", "Agnostic configuration update")

	plugin.Handle("config:update", handleUpdateConfig)

	if err := plugin.Run(); err != nil {
		plugin.Log(LogLevelError, "Plugin failed: "+err.Error())
	}
}

func handleUpdateConfig(msg AlloyMessage) AlloyMessage {
	plugin.Log(LogLevelInfo, "Repository config updated")
	return plugin.Reply(msg, map[string]string{"status": "ok"})
}
