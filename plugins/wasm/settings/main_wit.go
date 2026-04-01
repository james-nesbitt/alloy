//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
)

type UserConfig struct {
	Theme   string            `json:"theme"`
	Editor  map[string]any    `json:"editor"`
	Plugins map[string]any    `json:"plugins"`
	Aliases map[string]string `json:"aliases"`
}

var plugin *Plugin

func main() {
	plugin = NewPlugin("settings").
		WithMetadata(
			"Settings Manager",
			"Manages user preferences and global configuration",
			"0.1.0",
			"Alloy Team",
		).
		WithTags("settings", "config", "ui").
		WithCapability("get", "Get current user configuration").
		WithCapability("set", "Update user configuration").
		WithCapability("set-theme", "Update the UI theme").WithShortcut("u t")

	plugin.Handle("get", handleGet)
	plugin.Handle("set", handleSet)
	plugin.Handle("set-theme", handleSetTheme)

	plugin.OnInit(func() error {
		plugin.Log("info", "Settings plugin initializing")
		return nil
	})

	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Settings plugin failed: "+err.Error())
	}
}

func handleGet(msg AlloyMessage) AlloyMessage {
	resp := plugin.Call(AlloyMessage{
		Id:      "get-config-" + fmt.Sprint(msg.Timestamp),
		MsgType: "request",
		Method:  "project:get-composed-workspace",
		Sender:  "settings",
		Target:  Some("project"),
	})

	var data struct {
		UserConfig json.RawMessage `json:"user_config"`
	}
	if err := json.Unmarshal(resp.Payload, &data); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_project_response")
	}

	return plugin.Reply(msg, data.UserConfig)
}

func handleSet(msg AlloyMessage) AlloyMessage {
	plugin.RouteMessage(AlloyMessage{
		Id:      "set-config-" + fmt.Sprint(msg.Timestamp),
		MsgType: "request",
		Method:  "project:update-user-config",
		Sender:  "settings",
		Target:  Some("project"),
		Payload: msg.Payload,
	})

	// Broadcast theme change if present
	var config map[string]any
	if err := json.Unmarshal(msg.Payload, &config); err == nil {
		if theme, ok := config["theme"].(string); ok {
			broadcastTheme(theme)
		}
	}

	return plugin.Reply(msg, map[string]string{"status": "ok"})
}

func handleSetTheme(msg AlloyMessage) AlloyMessage {
	var theme string
	if err := json.Unmarshal(msg.Payload, &theme); err != nil {
		theme = string(msg.Payload)
	}

	// For Phase 10: We just broadcast the theme change.
	// In a complete version, we'd persist it to the user config via project plugin.
	broadcastTheme(theme)
	return plugin.Reply(msg, map[string]string{"status": "theme_updated", "theme": theme})
}

func broadcastTheme(theme string) {
	evtPayload, _ := json.Marshal(map[string]any{
		"topic": "system:theme-changed",
		"data":  map[string]string{"theme": theme},
	})
	plugin.RouteMessage(AlloyMessage{
		Id:      "theme-change-evt",
		MsgType: "request", // Events service expects requests for publishing
		Method:  "publish",
		Sender:  "settings",
		Target:  Some("events"),
		Payload: evtPayload,
	})
}
