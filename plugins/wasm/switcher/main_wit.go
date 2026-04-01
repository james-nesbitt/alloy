//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"time"
)

type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

var (
	plugin     *Plugin
	workspaces []Workspace
)

func main() {
	plugin = NewPlugin("switcher").
		WithMetadata(
			"Project Switcher",
			"Allows switching between different project contexts",
			"0.1.0",
			"Alloy Team",
		).
		WithCapability("switcher:list-projects", "List available projects").
		WithCapability("switcher:switch", "Switch active project context").
		WithShortcut("p s")

	plugin.Handle("switcher:list-projects", handleListProjects)
	plugin.Handle("switcher:switch", handleSwitch)

	plugin.OnInit(func() error {
		plugin.Log("info", "Project Switcher initializing")
		
		// Register a widget to show the current context
		plugin.RegisterWidget(AlloyWidget{
			Id:                "current-context",
			Title:             "Active Context",
			ContentType:       "text",
			Content:           []byte("Default"),
			RefreshIntervalMs: 0,
		})

		return nil
	})

	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Switcher plugin failed: "+err.Error())
	}
}

func handleListProjects(msg AlloyMessage) AlloyMessage {
	// Query the project plugin for workspaces
	resp := plugin.Call(AlloyMessage{
		Id:      "get-workspaces-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "request",
		Method:  "project:list-workspaces",
		Sender:  "switcher",
		Target:  Some("project"),
	})

	var data struct {
		Workspaces []Workspace `json:"workspaces"`
	}
	if err := json.Unmarshal(resp.Payload, &data); err != nil {
		return plugin.ErrorReply(msg, "failed_to_get_workspaces")
	}

	workspaces = data.Workspaces
	return plugin.Reply(msg, data)
}

func handleSwitch(msg AlloyMessage) AlloyMessage {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		// Try parsing as simple string
		var id string
		if err := json.Unmarshal(msg.Payload, &id); err == nil {
			req.ID = id
		} else {
			req.ID = string(msg.Payload)
		}
	}

	if req.ID == "" {
		return plugin.ErrorReply(msg, "invalid_request_missing_id")
	}

	// 1. Tell project plugin to set active workspace
	plugin.RouteMessage(AlloyMessage{
		Id:      "set-workspace-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "request",
		Method:  "project:set-workspace",
		Sender:  "switcher",
		Target:  Some("project"),
		Payload: msg.Payload,
	})

	// 2. Notify system of context change
	evtPayload, _ := json.Marshal(map[string]interface{}{
		"topic": "system:context-changed",
		"data":  map[string]string{"context_id": req.ID},
	})
	plugin.RouteMessage(AlloyMessage{
		Id:      "ctx-change-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "request",
		Method:  "events:publish",
		Sender:  "switcher",
		Target:  Some("events"),
		Payload: evtPayload,
	})

	// 3. Update own widget
	plugin.UpdateWidget("current-context", []byte(req.ID))

	return plugin.Reply(msg, map[string]string{"status": "ok", "context": req.ID})
}
