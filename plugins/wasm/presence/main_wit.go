//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/build/gen/bindings/guest"
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// UserPresence represents a user's status in a workspace.
type UserPresence struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"` // online, idle, away, dnd
	Workspace string `json:"workspace"`
	LastSeen  int64  `json:"last_seen"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

var (
	plugin   *Plugin
	presence = make(map[string]UserPresence)
)

func main() {
	plugin = NewPlugin("presence").
		WithMetadata(
			"Presence Manager",
			"Tracks user status across the workspace",
			"0.1.0",
			"Alloy Team",
		).
		WithTags("collaboration", "presence", "team").
		WithCapability("heartbeat", "Update user presence").
		WithCapability("list", "List all present users").WithShortcut("p l").
		WithCapability("get", "Get presence for a specific user")

	plugin.Handle("heartbeat", handleHeartbeat)
	plugin.Handle("list", handleList)
	plugin.Handle("get", handleGet)

	plugin.OnInit(func() error {
		plugin.Log("info", "Presence manager initializing")

		// Register a dashboard widget
		plugin.RegisterWidget(AlloyWidget{
			Id:                "presence-summary",
			Title:             "Team Presence",
			ContentType:       "text",
			Content:           []byte("No users online"),
			RefreshIntervalMs: 5000,
		})

		// Background cleanup of stale presence
		go func() {
			for {
				time.Sleep(10 * time.Second)
				now := time.Now().Unix()
				changed := false
				for id, p := range presence {
					if now-p.LastSeen > 60 { // 1-minute timeout
						delete(presence, id)
						changed = true
						plugin.Log("info", "User went offline: "+id)
						
						// Publish offline event
						publishPresenceEvent(id, "offline")
					}
				}

				if changed {
					updateDashboard()
				}
			}
		}()

		return nil
	})

	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

func handleHeartbeat(msg AlloyMessage) AlloyMessage {
	var req UserPresence
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_presence_data")
	}

	// Ensure ID is set (default to sender if empty)
	if req.ID == "" {
		req.ID = msg.Sender
	}
	if req.Name == "" {
		req.Name = req.ID
	}
	if req.Status == "" {
		req.Status = "online"
	}
	req.LastSeen = time.Now().Unix()

	_, exists := presence[req.ID]
	presence[req.ID] = req

	if !exists {
		plugin.Log("info", "User joined: "+req.ID)
		publishPresenceEvent(req.ID, "online")
	}

	updateDashboard()

	return plugin.Reply(msg, map[string]string{"status": "ok"})
}

func handleList(msg AlloyMessage) AlloyMessage {
	list := make([]UserPresence, 0, len(presence))
	for _, p := range presence {
		list = append(list, p)
	}
	return plugin.Reply(msg, list)
}

func handleGet(msg AlloyMessage) AlloyMessage {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request")
	}

	p, ok := presence[req.ID]
	if !ok {
		return plugin.ErrorReply(msg, "user_not_found")
	}

	return plugin.Reply(msg, p)
}

func updateDashboard() {
	if len(presence) == 0 {
		plugin.UpdateWidget("presence-summary", []byte("No users online"))
		return
	}

	var lines []string
	for _, p := range presence {
		icon := "●"
		if p.Status == "away" || p.Status == "idle" {
			icon = "○"
		} else if p.Status == "dnd" {
			icon = "×"
		}
		lines = append(lines, fmt.Sprintf("%s %s (%s)", icon, p.Name, p.Status))
	}
	plugin.UpdateWidget("presence-summary", []byte(strings.Join(lines, "\n")))
}

func publishPresenceEvent(userID string, eventType string) {
	evt := map[string]interface{}{
		"user_id":   userID,
		"event":     eventType,
		"timestamp": time.Now().Unix(),
	}
	
	// Create the event payload as expected by the events plugin
	payload, _ := json.Marshal(map[string]interface{}{
		"topic": "presence:" + eventType,
		"data":  evt,
	})
	
	plugin.RouteMessage(AlloyMessage{
		Id:      fmt.Sprintf("presence-evt-%d", time.Now().UnixNano()),
		MsgType: "request",
		Method:  "publish",
		Sender:  "presence",
		Target:  Some("events"),
		Payload: payload,
	})
}
