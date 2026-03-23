//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/build/gen/bindings/guest"
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
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
	Layout      json.RawMessage `json:"layout,omitempty"`
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
	plugin   *Plugin
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

func (m *Project) toDashboardTile() map[string]interface{} {
	content := []string{
		fmt.Sprintf("Name: %s", m.Name),
		fmt.Sprintf("ID: %s", m.ID),
	}
	if m.Description != "" {
		content = append(content, m.Description)
	}
	content = append(content, fmt.Sprintf("Buffers: %d", len(m.Buffers)))
	content = append(content, fmt.Sprintf("Channels: %d", len(m.Channels)))

	status := "Active"
	if !m.Active {
		status = "Inactive"
	}

	return map[string]interface{}{
		"title":   "Project Status",
		"content": content,
		"status":  status,
		"actions": []string{"Open", "Create"},
	}
}

func emitDashboardUpdate() {
	var active *Project
	for _, p := range projects {
		if p.Active {
			active = p
			break
		}
	}

	if active == nil {
		active = &Project{Name: "No Active Project", ID: "none", Active: false}
	}

	payload, _ := json.Marshal(active.toDashboardTile())
	plugin.RouteMessage(AlloyMessage{
		MsgType: "event",
		Method:  "dashboard-update",
		Sender:  "project",
		Payload: payload,
	})
}

func main() {
	// Create a new WIT-based plugin
	plugin = NewPlugin("project").
		WithMetadata(
			"Project Manager", 
			"Manages projects and their associated resources",
			"0.1.0", 
			"Alloy Team",
		).
		WithTags("project", "management", "organization").
		WithCapability("project:create", "Create a new project").
		WithCapability("project:list", "List all projects").WithShortcut("p l").
		WithCapability("project:get_active", "Get current active project").
		WithCapability("project:add-buffer", "Add a buffer to a project").
		WithCapability("project:add-channel", "Add a chat channel to a project").
		WithCapability("project:import", "Import a workspace from a path").
		WithCapability("project:open", "Open a project").WithShortcut("p o").
		WithCapability("project:save", "Save projects to durable store").
		WithCapability("project:discover", "Automatically detect and register workspaces").WithShortcut("p d").
		WithCapability("project:list-workspaces", "List all workspaces").WithShortcut("p p").
		WithCapability("project:set-workspace", "Switch active workspace")

	// Set up message handlers
	plugin.Handle("project:create", handleCreate)
	plugin.Handle("project:get_active", handleActive)
	plugin.Handle("project:list", handleList)
	plugin.Handle("project:add-buffer", handleAddBuffer)
	plugin.Handle("project:add-channel", handleAddChannel)
	plugin.Handle("project:open", handleOpen)
	plugin.Handle("project:save", handleSave)
	plugin.Handle("project:import", handleImport)
	plugin.Handle("project:discover", handleDiscover)
	plugin.Handle("project:list-workspaces", handleListWorkspaces)
	plugin.Handle("project:set-workspace", handleSetWorkspace)

	// Backward compatibility handlers
	plugin.Handle("create", handleCreate)
	plugin.Handle("get_active", handleActive)
	plugin.Handle("list", handleList)
	plugin.Handle("open", handleOpen)
	plugin.Handle("list-workspaces", handleListWorkspaces)
	plugin.Handle("set-workspace", handleSetWorkspace)

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "Project manager initializing")

		// Register in the component registry
		plugin.RegisterCapability(AlloyCapability{
			Method:      "project:get_active",
			Description: "Get the current active project",
		})

		// Automatically discover workspaces on start
		go discoverWorkspaces("/")

		// Restore from KV on startup
		if data, err := store.Get("all"); err == nil {
			projects = data
			plugin.Log("info", fmt.Sprintf("Restored %d projects", len(projects)))
			emitDashboardUpdate()
		}
		return nil
	})

	// Run the plugin
	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

// handleCreate handles project creation requests.
func handleCreate(msg AlloyMessage) AlloyMessage {
	var req ProjectCreateRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request: "+err.Error())
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

	// Update dashboard
	emitDashboardUpdate()

	plugin.Log("info", fmt.Sprintf("Created project: %s (%s)", proj.Name, proj.ID))

	return plugin.Reply(msg, proj)
}

// handleActive handles requests for the active project.
func handleActive(msg AlloyMessage) AlloyMessage {
	var active *Project
	for _, proj := range projects {
		if proj.Active {
			active = proj
			break
		}
	}

	if active == nil {
		return plugin.ErrorReply(msg, "no_active_project")
	}

	return plugin.Reply(msg, active)
}

// handleList handles requests to list all projects.
func handleList(msg AlloyMessage) AlloyMessage {
	list := make([]*Project, 0, len(projects))
	for _, proj := range projects {
		list = append(list, proj)
	}

	return plugin.Reply(msg, map[string]interface{}{
		"projects": list,
	})
}

// handleAddBuffer handles requests to add a buffer to a project.
func handleAddBuffer(msg AlloyMessage) AlloyMessage {
	var req ProjectAddResourceRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request: "+err.Error())
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
		return plugin.ErrorReply(msg, "project_not_found")
	}

	proj.Buffers = append(proj.Buffers, req.ResourceID)
	saveProjects()

	plugin.Log("info", fmt.Sprintf("Added buffer %s to project %s", req.ResourceID, proj.Name))

	return plugin.Reply(msg, map[string]string{"status": "ok"})
}

// handleAddChannel handles requests to add a channel to a project.
func handleAddChannel(msg AlloyMessage) AlloyMessage {
	var req ProjectAddResourceRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request: "+err.Error())
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
		return plugin.ErrorReply(msg, "project_not_found")
	}

	proj.Channels = append(proj.Channels, req.ResourceID)
	saveProjects()

	plugin.Log("info", fmt.Sprintf("Added channel %s to project %s", req.ResourceID, proj.Name))

	return plugin.Reply(msg, map[string]string{"status": "ok"})
}

// handleImport handles project imports from JSON payloads.
func handleImport(msg AlloyMessage) AlloyMessage {
	var proj Project
	if err := json.Unmarshal(msg.Payload, &proj); err != nil {
		return plugin.ErrorReply(msg, "invalid_project_json: "+err.Error())
	}

	if proj.ID == "" {
		proj.ID = fmt.Sprintf("proj-%d", time.Now().UnixNano())
	}

	projects[proj.ID] = &proj
	saveProjects()

	plugin.Log("info", fmt.Sprintf("Imported project: %s (%s)", proj.Name, proj.ID))

	return plugin.Reply(msg, proj)
}

// handleOpen handles requests to open a project.
func handleOpen(msg AlloyMessage) AlloyMessage {
	var req ProjectOpenRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request: "+err.Error())
	}

	proj, ok := projects[req.ID]
	if !ok {
		return plugin.ErrorReply(msg, "project_not_found")
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

	// Update Dashboard
	emitDashboardUpdate()

	plugin.Log("info", fmt.Sprintf("Opened project: %s (%s)", proj.Name, proj.ID))

	return plugin.Reply(msg, proj)
}

// handleSave handles requests to save projects.
func handleSave(msg AlloyMessage) AlloyMessage {
	saveProjects()
	return plugin.Reply(msg, map[string]string{"status": "saved"})
}

// handleDiscover initiates a filesystem scan for workspaces.
func handleDiscover(msg AlloyMessage) AlloyMessage {
	go discoverWorkspaces("/")
	return plugin.Reply(msg, map[string]string{"status": "discovery-initiated"})
}

// discoverWorkspaces scans the filesystem for .alloy/workspace.json files.
func discoverWorkspaces(root string) {
	plugin.Log("info", "Starting workspace discovery in: "+root)
	
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue scanning even if one branch fails
		}
		
		if info.IsDir() && info.Name() == ".alloy" {
			workspaceFile := filepath.Join(path, "workspace.json")
			if _, err := os.Stat(workspaceFile); err == nil {
				registerWorkspaceFromFile(path, workspaceFile)
			}
			return filepath.SkipDir // Don't scan inside .alloy
		}
		
		return nil
	})
	
	plugin.Log("info", "Workspace discovery complete")
}

// registerWorkspaceFromFile reads a workspace file and registers it with the host.
func registerWorkspaceFromFile(alloyDir, workspaceFile string) {
	data, err := os.ReadFile(workspaceFile)
	if err != nil {
		plugin.Log("error", "Failed to read workspace file: "+err.Error())
		return
	}
	
	// Temporarily unmarshal into a Project to get the layout
	var proj Project
	if err := json.Unmarshal(data, &proj); err != nil {
		plugin.Log("error", "Failed to parse workspace file as project: "+err.Error())
		return
	}

	var ws AlloyWorkspace
	if err := json.Unmarshal(data, &ws); err != nil {
		plugin.Log("error", "Failed to parse workspace file: "+err.Error())
		return
	}
	
	// If ID or Path are not set in the JSON, derive them from the location
	if ws.Id == "" {
		ws.Id = filepath.Base(filepath.Dir(alloyDir))
	}
	if ws.Path == "" {
		ws.Path = filepath.Dir(alloyDir)
	}

	// Set layout if present in the project struct
	if len(proj.Layout) > 0 {
		ws.Layout = Some(string(proj.Layout))
	}
	
	plugin.Log("info", fmt.Sprintf("Registering discovered workspace: %s (%s)", ws.Name, ws.Path))
	plugin.RegisterWorkspace(ws)
}

// handleListWorkspaces returns a list of all workspaces from the host registry.
func handleListWorkspaces(msg AlloyMessage) AlloyMessage {
	workspaces := plugin.ListWorkspaces()
	return plugin.Reply(msg, map[string]interface{}{
		"workspaces": workspaces,
	})
}

// handleSetWorkspace switches the active workspace in the host registry.
func handleSetWorkspace(msg AlloyMessage) AlloyMessage {
	var id string
	if err := json.Unmarshal(msg.Payload, &id); err != nil {
		// Try unmarshalling as a map if it's from the TUI form
		var req map[string]string
		if err := json.Unmarshal(msg.Payload, &req); err == nil {
			id = req["id"]
		} else {
			id = string(msg.Payload)
		}
	}
	plugin.SetActiveWorkspace(id)

	// Notify about workspace change
	var active *AlloyWorkspace
	workspaces := plugin.ListWorkspaces()
	for _, ws := range workspaces {
		if ws.Id == id {
			active = &ws
			break
		}
	}

	if active != nil {
		evtPayload, _ := json.Marshal(map[string]interface{}{
			"topic": "workspace:opened",
			"data":  active,
		})
		plugin.RouteMessage(AlloyMessage{
			MsgType: "event",
			Method:  "publish",
			Sender:  "project",
			Target:  Some("events"),
			Payload: evtPayload,
		})
	}

	return plugin.Reply(msg, map[string]string{"status": "ok", "workspace": id})
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

// notifyProjectOpened notifies about a project being opened via events service.
func notifyProjectOpened(proj *Project) {
	evtPayload, _ := json.Marshal(map[string]interface{}{
		"topic": "project:opened",
		"data":  proj,
	})

	plugin.RouteMessage(AlloyMessage{
		MsgType: "request", // or event
		Method:  "publish",
		Sender:  "project",
		Target:  Some("events"),
		Payload: evtPayload,
	})
}
