package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

type Project struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Buffers     []string `json:"buffers,omitempty"`
	Channels    []string `json:"channels,omitempty"`
	Files       []string `json:"files,omitempty"`
	Active      bool     `json:"active"`
}

var projects = make(map[string]*Project)

func init() {
	wasm.SetHandler(handleMessage)
	wasm.SetCapabilities([]wasm.Capability{
		{Method: "create", Description: "Create a new project", Shortcut: "p c", Annotations: map[string]string{"group": "project"}},
		{Method: "list", Description: "List all projects", Shortcut: "p l", Annotations: map[string]string{"group": "project"}},
		{Method: "active", Description: "Get current active project", Shortcut: "p a", Annotations: map[string]string{"group": "project"}},
		{Method: "add:buffer", Description: "Add a buffer to the current project", Shortcut: "p a b", Annotations: map[string]string{"group": "project"}},
		{Method: "add:channel", Description: "Add a chat channel to the current project", Shortcut: "p a c", Annotations: map[string]string{"group": "project"}},
		{Method: "open", Description: "Open a project", Shortcut: "p o", Annotations: map[string]string{"group": "project"}},
		{Method: "save", Description: "Save projects to durable store", Shortcut: "p s", Annotations: map[string]string{"group": "project"}},
	})
}

func main() {
	// Restore from KV on startup
	if data := wasm.KVGet("all-projects"); data != nil {
		json.Unmarshal(data, &projects)
	}

	wasm.SleepForever()
}

func handleMessage(msg wasm.Message) wasm.Message {
	switch msg.Method {
	case "create":
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		json.Unmarshal(msg.Payload, &req)

		id := fmt.Sprintf("proj-%d", time.Now().UnixNano())
		p := &Project{
			ID:          id,
			Name:        req.Name,
			Description: req.Description,
		}
		projects[id] = p
		saveProjects() // Persistent saving

		return wasm.Message{
			ID:     msg.ID + "-resp",
			Type:   "response",
			Sender: "plugin-project-manager",
			Target: msg.Sender,
			Payload: mustMarshal(p),
		}

	case "active":
		var active *Project
		for _, p := range projects {
			if p.Active {
				active = p
				break
			}
		}
		if active == nil {
			return errorResponse(msg, "no active project")
		}
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-project-manager", Target: msg.Sender,
			Payload: mustMarshal(active),
		}

	case "list":
		list := make([]*Project, 0, len(projects))
		for _, p := range projects {
			list = append(list, p)
		}
		return wasm.Message{
			ID:     msg.ID + "-resp",
			Type:   "response",
			Sender: "plugin-project-manager",
			Target: msg.Sender,
			Payload: mustMarshal(map[string]interface{}{
				"projects": list,
			}),
		}

	case "add:buffer":
		var req struct {
			ProjectID string `json:"project_id"`
			BufferID  string `json:"buffer_id"`
		}
		json.Unmarshal(msg.Payload, &req)
		
		targetID := req.ProjectID
		if targetID == "" {
			for _, p := range projects {
				if p.Active {
					targetID = p.ID
					break
				}
			}
		}

		p, ok := projects[targetID]
		if !ok { return errorResponse(msg, "project not found") }
		p.Buffers = append(p.Buffers, req.BufferID)
		saveProjects()
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-project-manager", Target: msg.Sender,
			Payload: []byte(`{"status":"ok"}`),
		}

	case "add:channel":
		var req struct {
			ProjectID string `json:"project_id"`
			Channel string `json:"channel"`
		}
		json.Unmarshal(msg.Payload, &req)

		targetID := req.ProjectID
		if targetID == "" {
			for _, p := range projects {
				if p.Active {
					targetID = p.ID
					break
				}
			}
		}

		p, ok := projects[targetID]
		if !ok { return errorResponse(msg, "project not found") }
		p.Channels = append(p.Channels, req.Channel)
		saveProjects()
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-project-manager", Target: msg.Sender,
			Payload: []byte(`{"status":"ok"}`),
		}

	case "open":
		var req struct { ID string `json:"id"` }
		json.Unmarshal(msg.Payload, &req)
		p, ok := projects[req.ID]
		if !ok { return errorResponse(msg, "project not found") }
		
		for _, proj := range projects { proj.Active = false }
		p.Active = true
		saveProjects()
		
		// Notify frontends via event
		publishEvent("project:opened", p)
		
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-project-manager", Target: msg.Sender,
			Payload: mustMarshal(p),
		}

	case "save":
		saveProjects()
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-project-manager", Target: msg.Sender,
			Payload: []byte(`{"status":"saved"}`),
		}

	default:
		return wasm.Message{}
	}
}

func saveProjects() {
	data, _ := json.Marshal(projects)
	wasm.KVSet("all-projects", data)
}

func publishEvent(topic string, data any) {
	payload, _ := json.Marshal(struct {
		Topic string `json:"topic"`
		Data  any    `json:"data"`
	}{
		Topic: topic,
		Data:  data,
	})
	wasm.RouteMessage(wasm.Message{
		ID:        fmt.Sprintf("evt-%s-%d", topic, time.Now().UnixNano()),
		Type:      "event",
		Sender:    "plugin-project-manager",
		Target:    "plugin-events",
		Method:    "publish",
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	})
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func errorResponse(msg wasm.Message, err string) wasm.Message {
	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-project-manager",
		Target: msg.Sender,
		Payload: []byte(fmt.Sprintf(`{"error":"%s"}`, err)),
	}
}
