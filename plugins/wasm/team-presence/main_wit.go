//go:build wasip1 || wasm

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/james-nesbitt/alloy/pkg/wasm/guest"
	wit "github.com/james-nesbitt/alloy/build/gen/bindings/guest"
)

type Presence struct {
	User      string `json:"user"`
	Status    string `json:"status"`
	LastSeen  int64  `json:"last_seen"`
	Client    string `json:"client"`
	ProjectID string `json:"project_id,omitempty"`
}

type DashboardTile struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Content   []string `json:"content"`
	Status    string   `json:"status"`
	Actions   []string `json:"actions"`
	Timestamp int64    `json:"timestamp"`
}

var (
	presenceMap = make(map[string]Presence)
)

func main() {
	p := guest.NewPlugin("team-presence")

	p.OnInit(func() error {
		// Subscribe to presence events
		subPayload, _ := json.Marshal(map[string]string{"topic": "presence:heartbeat"})
		p.RouteMessage(wit.AlloyMessage{
			Id:      "sub-presence",
			MsgType: "request",
			Sender:  "team-presence",
			Target:  wit.Some("events"),
			Method:  "subscribe",
			Payload: subPayload,
		})
		return nil
	})

	p.RegisterMethod("list", "List all active users", func(msg guest.Message) *guest.Message {
		var list []Presence
		for _, pres := range presenceMap {
			// Filter out stale entries (> 5 mins)
			if time.Now().Unix()-pres.LastSeen < 300 {
				list = append(list, pres)
			}
		}
		
		sort.Slice(list, func(i, j int) bool {
			return list[i].User < list[j].User
		})

		payload, _ := json.Marshal(list)
		return &guest.Message{
			ID:      msg.ID + "-resp",
			Method:  "presence:list",
			Payload: payload,
			Target:  msg.Sender,
		}
	})

	// Handle the heartbeat event
	p.Handle("presence:heartbeat", func(rawMsg wit.AlloyMessage) wit.AlloyMessage {
		var event struct {
			Topic string   `json:"topic"`
			Data  Presence `json:"data"`
		}
		if err := json.Unmarshal(rawMsg.Payload, &event); err == nil {
			pres := event.Data
			if pres.LastSeen == 0 {
				pres.LastSeen = time.Now().Unix()
			}
			presenceMap[pres.User] = pres
			
			// Update the dashboard immediately on change
			updateDashboard(p)
		}
		return wit.AlloyMessage{} // No response for events
	})

	p.Log(guest.LogLevelInfo, "Team Presence plugin initialized")
	p.Serve()
}

func updateDashboard(p *guest.Plugin) {
	var entries []string
	onlineCount := 0
	
	users := make([]string, 0, len(presenceMap))
	for u := range presenceMap {
		users = append(users, u)
	}
	sort.Strings(users)

	now := time.Now().Unix()
	for _, u := range users {
		pres := presenceMap[u]
		if now-pres.LastSeen > 300 {
			continue
		}
		
		dot := "●"
		if pres.Status == "away" {
			dot = "○"
		} else if pres.Status == "busy" {
			dot = "◌"
		}
		
		entries = append(entries, fmt.Sprintf("%s %s (%s)", dot, pres.User, pres.Client))
		onlineCount++
	}

	if len(entries) == 0 {
		entries = append(entries, "No one else is online")
	}

	tile := DashboardTile{
		ID:      "team-presence",
		Title:   "Team Presence",
		Content: entries,
		Status:  fmt.Sprintf("%d Online", onlineCount),
		Actions: []string{"Invite", "Status"},
		Timestamp: now,
	}

	payload, _ := json.Marshal(tile)
	p.RouteMessage(wit.AlloyMessage{
		Id:      "dash-update-presence",
		MsgType: "request",
		Sender:  "team-presence",
		Target:  wit.Some("events"),
		Method:  "publish",
		Payload: func() []byte {
			ev := map[string]any{
				"topic": "dashboard-update",
				"data":  tile,
			}
			b, _ := json.Marshal(ev)
			return b
		}(),
	})
	
	// Also send directly to frontends as a dashboard update if they are listening
	p.RouteMessage(wit.AlloyMessage{
		Id:      "dash-direct-presence",
		MsgType: "request",
		Sender:  "team-presence",
		Target:  wit.None[string](), // Broadcast-ish or routed by method
		Method:  "dashboard-update",
		Payload: payload,
	})
}
