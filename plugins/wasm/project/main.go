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

var (
	projects = make(map[string]*Project)
	store = wasm.NewKVStore[map[string]*Project]("project-manager")
)

func main() {
	p := wasm.New("plugin-project-manager").
		WithCapability("create", "Create a new project", "p c").
		WithCapability("list", "List all projects", "p l").
		WithCapability("active", "Get current active project", "p a").
		WithCapability("add:buffer", "Add a buffer to the current project", "p a b").
		WithCapability("add:channel", "Add a chat channel to the current project", "p a c").
		WithCapability("open", "Open a project", "p o").
		WithCapability("save", "Save projects to durable store", "p s").
		OnInit(func() error {
			// Restore from KV on startup
			if data, err := store.Get("all"); err == nil {
				projects = data
			}
			return nil
		})

	p.Handle("create", func(msg wasm.Message) wasm.Message {
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		_ = json.Unmarshal(msg.Payload, &req)

		id := fmt.Sprintf("proj-%d", time.Now().UnixNano())
		proj := &Project{
			ID:          id,
			Name:        req.Name,
			Description: req.Description,
		}
		projects[id] = proj
		saveProjects() // Persistent saving

		return wasm.Reply(msg, proj)
	})

	p.Handle("active", func(msg wasm.Message) wasm.Message {
		var active *Project
		for _, proj := range projects {
			if proj.Active {
				active = proj
				break
			}
		}
		if active == nil {
			return wasm.ErrorReply(msg, "no active project")
		}
		return wasm.Reply(msg, active)
	})

	p.Handle("list", func(msg wasm.Message) wasm.Message {
		list := make([]*Project, 0, len(projects))
		for _, proj := range projects {
			list = append(list, proj)
		}
		return wasm.Reply(msg, map[string]interface{}{
			"projects": list,
		})
	})

	p.Handle("add:buffer", func(msg wasm.Message) wasm.Message {
		var req struct {
			ProjectID string `json:"project_id"`
			BufferID  string `json:"buffer_id"`
		}
		_ = json.Unmarshal(msg.Payload, &req)
		
		targetID := req.ProjectID
		if targetID == "" {
			for _, proj := range projects {
				if proj.Active {
					targetID = proj.ID
					break
				}
			}
		}

		proj, ok := projects[targetID]
		if !ok { return wasm.ErrorReply(msg, "project not found") }
		proj.Buffers = append(proj.Buffers, req.BufferID)
		saveProjects()
		return wasm.Reply(msg, map[string]string{"status": "ok"})
	})

	p.Handle("add:channel", func(msg wasm.Message) wasm.Message {
		var req struct {
			ProjectID string `json:"project_id"`
			Channel string `json:"channel"`
		}
		_ = json.Unmarshal(msg.Payload, &req)

		targetID := req.ProjectID
		if targetID == "" {
			for _, proj := range projects {
				if proj.Active {
					targetID = proj.ID
					break
				}
			}
		}

		proj, ok := projects[targetID]
		if !ok { return wasm.ErrorReply(msg, "project not found") }
		proj.Channels = append(proj.Channels, req.Channel)
		saveProjects()
		return wasm.Reply(msg, map[string]string{"status": "ok"})
	})

	p.Handle("open", func(msg wasm.Message) wasm.Message {
		var req struct { ID string `json:"id"` }
		_ = json.Unmarshal(msg.Payload, &req)
		proj, ok := projects[req.ID]
		if !ok { return wasm.ErrorReply(msg, "project not found") }
		
		for _, pr := range projects { pr.Active = false }
		proj.Active = true
		saveProjects()
		
		// Notify frontends via event
		p.Events.Emit("project:opened", proj)
		
		return wasm.Reply(msg, proj)
	})

	p.Handle("save", func(msg wasm.Message) wasm.Message {
		saveProjects()
		return wasm.Reply(msg, map[string]string{"status": "saved"})
	})

	p.Run()
}

func saveProjects() {
	_ = store.Set("all", projects)

	for _, pr := range projects {
		if pr.Active {
			pData, _ := json.Marshal(pr)
			wasm.KVSet("shared:active-project", pData)
			return
		}
	}
	wasm.KVSet("shared:active-project", nil)
}
