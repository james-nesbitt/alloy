//go:build wasip1 || wasm

package main

import (
	"encoding/json"
	"fmt"
	"time"

	"./wit"
)

// Project represents a project in the system.
type Project struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Buffers     []string `json:"buffers,omitempty"`
	Channels    []string `json:"channels,omitempty"`
	Files       []string `json:"files,omitempty"`
	Active      bool     `json:"active"`
}

// ProjectCreateRequest represents a request to create a project.
type ProjectCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ProjectAddResourceRequest represents a request to add a resource to a project.
type ProjectAddResourceRequest struct {
	ProjectID string `json:"project_id"`
	ResourceID string `json:"resource_id"`
}

// ProjectOpenRequest represents a request to open a project.
type ProjectOpenRequest struct {
	ID string `json:"id"`
}

var (
	projects = make(map[string]*Project)
	plugin   *guest.Plugin
	store    = NewKVStore[map[string]*Project]("project-manager")
)

// KVStore provides type-safe KV storage.
type KVStore[T any] struct {
	prefix string
}

// NewKVStore creates a new KVStore instance.
func NewKVStore[T any](prefix string) *KVStore[T] {
	return &KVStore[T]{prefix: prefix}
}

// Get retrieves a value from KV storage.
func (s *KVStore[T]) Get(key string) (T, error) {
	var result T
	data, ok := plugin.KVGet(s.prefix + ":" + key)
	if !ok || data == nil {
		return result, fmt.Errorf("not found")
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	return result, nil
}

// Set stores a value in KV storage.
func (s *KVStore[T]) Set(key string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !plugin.KVSet(s.prefix+":"+key, data) {
		return fmt.Errorf("failed to set value")
	}
	return nil
}

func main() {
	// Create a new WIT-based plugin
	plugin = guest.NewPlugin("project-manager").
		WithMetadata(
			"Project Manager", 
			"Manages projects and their associated resources",
			"0.1.0", 
			"Alloy Team",
		).
		WithTags("project", "management", "organization").
		WithCapability("create", "Create a new project").
		WithCapability("list", "List all projects").
		WithCapability("active", "Get current active project").
		WithCapability("add:buffer", "Add a buffer to a project").
		WithCapability("add:channel", "Add a chat channel to a project").
		WithCapability("open", "Open a project").
		WithCapability("save", "Save projects to durable store")

	// Set up message handlers
	plugin.Handle("create", handleCreate)
	plugin.Handle("active", handleActive)
	plugin.Handle("list", handleList)
	plugin.Handle("add:buffer", handleAddBuffer)
	plugin.Handle("add:channel", handleAddChannel)
	plugin.Handle("open", handleOpen)
	plugin.Handle("save", handleSave)

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "Project manager initializing")
		// Restore from KV on startup
		if data, err := store.Get("all"); err == nil {
			projects = data
			plugin.Log("info", fmt.Sprintf("Restored %d projects", len(projects)))
		}
		return nil
	})

	// Run the plugin
	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

// handleCreate handles project creation requests.
func handleCreate(msg guest.AlloyMessage) guest.AlloyMessage {
	var req ProjectCreateRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return guest.ErrorReply(msg, "invalid_request: "+err.Error())
	}

	id := fmt.Sprintf("proj-%d", time.Now().UnixNano())
	proj := &Project{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
	}
	projects[id] = proj

	// Save projects to persistent storage
	saveProjects()

	plugin.Log("info", fmt.Sprintf("Created project: %s (%s)", proj.Name, proj.ID))

	return guest.Reply(msg, proj)
}

// handleActive handles requests for the active project.
func handleActive(msg guest.AlloyMessage) guest.AlloyMessage {
	var active *Project
	for _, proj := range projects {
		if proj.Active {
			active = proj
			break
		}
	}

	if active == nil {
		return guest.ErrorReply(msg, "no_active_project")
	}

	return guest.Reply(msg, active)
}

// handleList handles requests to list all projects.
func handleList(msg guest.AlloyMessage) guest.AlloyMessage {
	list := make([]*Project, 0, len(projects))
	for _, proj := range projects {
		list = append(list, proj)
	}

	return guest.Reply(msg, map[string]interface{}{
		"projects": list,
	})
}

// handleAddBuffer handles requests to add a buffer to a project.
func handleAddBuffer(msg guest.AlloyMessage) guest.AlloyMessage {
	var req ProjectAddResourceRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return guest.ErrorReply(msg, "invalid_request: "+err.Error())
	}

	targetID := req.ProjectID
	if targetID == "" {
		// Find active project if none specified
		for _, proj := range projects {
			if proj.Active {
				targetID = proj.ID
				break
			}
		}
	}

	proj, ok := projects[targetID]
	if !ok {
		return guest.ErrorReply(msg, "project_not_found")
	}

	proj.Buffers = append(proj.Buffers, req.ResourceID)
	saveProjects()

	plugin.Log("info", fmt.Sprintf("Added buffer %s to project %s", req.ResourceID, proj.Name))

	return guest.Reply(msg, map[string]string{"status": "ok"})
}

// handleAddChannel handles requests to add a channel to a project.
func handleAddChannel(msg guest.AlloyMessage) guest.AlloyMessage {
	var req ProjectAddResourceRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return guest.ErrorReply(msg, "invalid_request: "+err.Error())
	}

	targetID := req.ProjectID
	if targetID == "" {
		// Find active project if none specified
		for _, proj := range projects {
			if proj.Active {
				targetID = proj.ID
				break
			}
		}
	}

	proj, ok := projects[targetID]
	if !ok {
		return guest.ErrorReply(msg, "project_not_found")
	}

	proj.Channels = append(proj.Channels, req.ResourceID)
	saveProjects()

	plugin.Log("info", fmt.Sprintf("Added channel %s to project %s", req.ResourceID, proj.Name))

	return guest.Reply(msg, map[string]string{"status": "ok"})
}

// handleOpen handles requests to open a project.
func handleOpen(msg guest.AlloyMessage) guest.AlloyMessage {
	var req ProjectOpenRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return guest.ErrorReply(msg, "invalid_request: "+err.Error())
	}

	proj, ok := projects[req.ID]
	if !ok {
		return guest.ErrorReply(msg, "project_not_found")
	}

	// Deactivate all projects
	for _, pr := range projects {
		pr.Active = false
	}

	// Activate the requested project
	proj.Active = true
	saveProjects()

	// Notify about the project opening
	notifyProjectOpened(proj)

	plugin.Log("info", fmt.Sprintf("Opened project: %s (%s)", proj.Name, proj.ID))

	return guest.Reply(msg, proj)
}

// handleSave handles requests to save projects.
func handleSave(msg guest.AlloyMessage) guest.AlloyMessage {
	saveProjects()
	return guest.Reply(msg, map[string]string{"status": "saved"})
}

// saveProjects saves all projects to persistent storage.
func saveProjects() {
	// Save all projects
	if err := store.Set("all", projects); err != nil {
		plugin.Log("error", "Failed to save projects: "+err.Error())
	}

	// Save active project separately
	for _, pr := range projects {
		if pr.Active {
			pData, _ := json.Marshal(pr)
			plugin.KVSet("shared:active-project", pData)
			return
		}
	}

	// No active project
	plugin.KVSet("shared:active-project", nil)
}

// notifyProjectOpened notifies about a project being opened.
func notifyProjectOpened(proj *Project) {
	// Create event payload
	eventPayload, err := json.Marshal(map[string]interface{}{
		"event":   "project:opened",
		"project": proj,
	})
	if err != nil {
		plugin.Log("error", "Failed to marshal project opened event: "+err.Error())
		return
	}

	// Route the event
	plugin.RouteMessage(guest.AlloyMessage{
		ID:      "project-opened-" + fmt.Sprint(time.Now().UnixNano()),
		Method:  "event",
		Sender:  "project-manager",
		Payload: eventPayload,
	})
}