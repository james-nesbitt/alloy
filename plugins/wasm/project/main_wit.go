//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

// Workspace represents a discovered workspace.
type Workspace struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Path      string            `json:"path"`
	TeamID    string            `json:"team_id,omitempty"`
	Layout    string            `json:"layout,omitempty"`
	ViewState string            `json:"view_state,omitempty"` // Added for Target 3 persistence
	Metadata  map[string]string `json:"metadata,omitempty"`
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

// ProjectSecurityConfig defines roles and assignments for the project.
type ProjectSecurityConfig struct {
	Roles       map[string][]string `json:"roles"`
	Assignments map[string]string   `json:"assignments"`
}

var (
	projects   = make(map[string]*Project)
	workspaces = make(map[string]*Workspace)
	activeWorkspaceID string
	plugin     *Plugin
	store      = NewKVStore[map[string]*Project]("project-manager")
	wsStore    = NewKVStore[map[string]*Workspace]("workspace-manager")
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
		WithAnnotations("project:create", map[string]string{
			"group":  "project",
			"params": "name,description",
		}).
		WithCapability("project:list", "List all projects").WithShortcut("p l").
		WithCapability("project:get_active", "Get current active project").
		WithCapability("project:add-buffer", "Add a buffer to a project").
		WithCapability("project:add-channel", "Add a chat channel to a project").
		WithCapability("project:import", "Import a workspace from a path").
		WithCapability("project:open", "Open a project").WithShortcut("p o").
		WithAnnotations("project:open", map[string]string{
			"group":  "project",
			"params": "id",
		}).
		WithCapability("project:save", "Save projects to durable store").
		WithCapability("project:discover", "Automatically detect and register workspaces").WithShortcut("p d").
		WithCapability("project:list-workspaces", "List all workspaces").WithShortcut("p p").
		WithCapability("project:set-workspace", "Switch active workspace").
		WithAnnotations("project:set-workspace", map[string]string{
			"group":  "project",
			"params": "id",
		}).
		WithCapability("project:set-security", "Set security roles for a project").
		WithCapability("project:update-user-config", "Update global user configuration").
		WithCapability("project:update-layout", "Update active workspace layout").
		WithCapability("project:update-view-state", "Update active workspace UI persistence state").
		WithCapability("project:get-composed-workspace", "Get a unified view of the workspace").
		WithCapability("project:archive", "Archive current workspace to an .ark file").
		WithCapability("project:restore", "Restore workspace from an .ark file").
		WithCapability("project:list-archives", "List all workspace archives in the data directory").
		WithCapability("project:delete-archive", "Delete a workspace archive from the data directory")


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
	plugin.Handle("project:set-security", handleSetSecurity)
	plugin.Handle("project:update-user-config", handleUpdateUserConfig)
	plugin.Handle("project:update-layout", handleUpdateLayout)
	plugin.Handle("project:update-view-state", handleUpdateViewState)
	plugin.Handle("project:get-composed-workspace", handleGetComposedWorkspace)
	plugin.Handle("project:archive", handleArchive)
	plugin.Handle("project:restore", handleRestore)
	plugin.Handle("project:list-archives", handleListArchives)
	plugin.Handle("project:delete-archive", handleDeleteArchive)


	// Backward compatibility handlers
	plugin.Handle("create", handleCreate)
	plugin.Handle("get_active", handleActive)
	plugin.Handle("list", handleList)
	plugin.Handle("open", handleOpen)
	plugin.Handle("list-workspaces", handleListWorkspaces)
	plugin.Handle("set-workspace", handleSetWorkspace)
	plugin.Handle("update-user-config", handleUpdateUserConfig)
	plugin.Handle("get-composed-workspace", handleGetComposedWorkspace)
	plugin.Handle("list-archives", handleListArchives)
	plugin.Handle("delete-archive", handleDeleteArchive)


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
		if wsData, err := wsStore.Get("all"); err == nil {
			workspaces = wsData
			plugin.Log("info", fmt.Sprintf("Restored %d workspaces", len(workspaces)))
		}
		if activeID, ok := plugin.KVGet("active-workspace-id"); ok {
			activeWorkspaceID = string(activeID)
		}
		if ucData, ok := plugin.KVGet("user-config"); ok {
			currentUserConfig = ucData
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

// registerWorkspaceFromFile reads a workspace file and registers it internaly.
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

	var ws Workspace
	if err := json.Unmarshal(data, &ws); err != nil {
		plugin.Log("error", "Failed to parse workspace file: "+err.Error())
		return
	}
	
	// If ID or Path are not set in the JSON, derive them from the location
	if ws.ID == "" {
		ws.ID = filepath.Base(filepath.Dir(alloyDir))
	}
	if ws.Path == "" {
		ws.Path = filepath.Dir(alloyDir)
	}

	// Set layout if present in the project struct
	if len(proj.Layout) > 0 {
		ws.Layout = string(proj.Layout)
	}
	
	plugin.Log("info", fmt.Sprintf("Registering discovered workspace: %s (%s)", ws.Name, ws.Path))
	workspaces[ws.ID] = &ws
	saveWorkspaces()
}

// handleListWorkspaces returns a list of all workspaces.
func handleListWorkspaces(msg AlloyMessage) AlloyMessage {
	list := make([]*Workspace, 0, len(workspaces))
	for _, ws := range workspaces {
		list = append(list, ws)
	}
	return plugin.Reply(msg, map[string]interface{}{
		"workspaces": list,
	})
}

// handleSetWorkspace switches the active workspace.
func handleSetWorkspace(msg AlloyMessage) AlloyMessage {
	var id string
	if err := json.Unmarshal(msg.Payload, &id); err != nil {
		var req map[string]string
		if err := json.Unmarshal(msg.Payload, &req); err == nil {
			id = req["id"]
		} else {
			id = string(msg.Payload)
		}
	}
	activeWorkspaceID = id
	plugin.KVSet("active-workspace-id", []byte(id))

	active := workspaces[id]
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

// handleSetSecurity applies security configurations to the host IAM service.
func handleSetSecurity(msg AlloyMessage) AlloyMessage {
	var cfg ProjectSecurityConfig
	if err := json.Unmarshal(msg.Payload, &cfg); err != nil {
		return plugin.ErrorReply(msg, "invalid_security_config")
	}

	namespace := msg.ContextID()
	if namespace == "" {
		// FALLBACK: If not explicitly set, try to get from project-specific metadata
		// In bootstrap, we set it in the context manually
		nsRaw, ok := msg.GetMetadata("namespace")
		if ok {
			namespace = fmt.Sprintf("%v", nsRaw)
		}
	}

	if namespace == "" {
		return plugin.ErrorReply(msg, "missing_namespace_context")
	}

	plugin.Log("info", fmt.Sprintf("Applying security policy for namespace: %s", namespace))

	// For each assignment, map it to the requested role capabilities
	for actor, roleName := range cfg.Assignments {
		caps, exists := cfg.Roles[roleName]
		if !exists {
			plugin.Log("warn", "Assigned role not found: "+roleName)
			continue
		}

		// Grant to IAM service
		grantPayload, _ := json.Marshal(map[string]interface{}{
			"actor":        actor,
			"namespace":    namespace,
			"capabilities": caps,
		})

		plugin.RouteMessage(AlloyMessage{
			MsgType: "request",
			Sender:  "project",
			Target:  Some("iam"),
			Method:  "grant_namespace_role",
			Payload: grantPayload,
		})
	}

	return plugin.Reply(msg, map[string]string{"status": "applied"})
}

var (
	currentUserConfig json.RawMessage
)

func handleUpdateUserConfig(msg AlloyMessage) AlloyMessage {
	currentUserConfig = msg.Payload
	plugin.KVSet("user-config", currentUserConfig)
	return plugin.Reply(msg, map[string]string{"status": "updated"})
}

func handleUpdateLayout(msg AlloyMessage) AlloyMessage {
	if activeWorkspaceID == "" {
		return plugin.ErrorReply(msg, "no_active_workspace")
	}
	ws, ok := workspaces[activeWorkspaceID]
	if !ok {
		return plugin.ErrorReply(msg, "workspace_not_found")
	}
	ws.Layout = string(msg.Payload)
	saveWorkspaces()

	plugin.Log("info", "Updated layout for workspace: "+ws.Name)
	return plugin.Reply(msg, map[string]string{"status": "layout_updated"})
}

func handleUpdateViewState(msg AlloyMessage) AlloyMessage {
	if activeWorkspaceID == "" {
		return plugin.ErrorReply(msg, "no_active_workspace")
	}
	ws, ok := workspaces[activeWorkspaceID]
	if !ok {
		return plugin.ErrorReply(msg, "workspace_not_found")
	}
	ws.ViewState = string(msg.Payload)
	saveWorkspaces()

	// Optionally broadcast this update to other clients? 
	// (Avoid for now to prevent loops unless explicit)
	return plugin.Reply(msg, map[string]string{"status": "view_state_updated"})
}

func handleGetComposedWorkspace(msg AlloyMessage) AlloyMessage {
	var wsID string
	if len(msg.Payload) > 0 {
		var req struct{ ID string }
		if err := json.Unmarshal(msg.Payload, &req); err == nil {
			wsID = req.ID
		} else {
			wsID = string(msg.Payload)
		}
	}
	if wsID == "" {
		wsID = activeWorkspaceID
	}

	ws, ok := workspaces[wsID]
	if !ok {
		return plugin.ErrorReply(msg, "workspace_not_found")
	}

	// Fetch ALL plugins metadata from host to find matches
	metaResp := plugin.Call(AlloyMessage{
		Id:      "get-plugin-meta-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "request",
		Method:  "command-manager:discover",
		Sender:  "project",
		Target:  Some("command-manager"),
	})

	var discovery struct {
		Targets []struct{ ID string } `json:"targets"`
	}
	json.Unmarshal(metaResp.Payload, &discovery)
	
	// Simplify: just return the workspace and user config for the frontend to compose
	// Or we can do it here if we want to follow the "Composition Engine" pattern
	
	return plugin.Reply(msg, map[string]interface{}{
		"workspace":   ws,
		"user_config": currentUserConfig,
		"active_id":   activeWorkspaceID,
	})
}

// saveProjects saves all projects to persistent storage.
func saveProjects() {
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
	plugin.KVSet("shared:active-project", nil)
}

func saveWorkspaces() {
	if err := wsStore.Set("all", workspaces); err != nil {
		plugin.Log("error", "Failed to save workspaces: "+err.Error())
	}
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

// handleArchive creates a workspace archive (.ark)
func handleArchive(msg AlloyMessage) AlloyMessage {
	// 1. Discover all plugins
	plugin.Log("info", "Starting workspace archival...")
	
	// Fetch plugin metadata from host
	metaResp := plugin.Call(AlloyMessage{
		Id:      "get-plugin-meta-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "request",
		Method:  "command-manager:discover",
		Sender:  "project",
		Target:  Some("command-manager"),
	})
	
	type target struct {
		ID string `json:"id"`
	}
	var discovery struct {
		Targets []target `json:"targets"`
	}
	
	if metaResp.Method == "error" || len(metaResp.Payload) == 0 {
		plugin.Log("info", "Registry discovery failed or empty, using fallback")
		discovery.Targets = []target{{ID: "project"}, {ID: "buffer"}, {ID: "librarian"}}
	} else {
		if err := json.Unmarshal(metaResp.Payload, &discovery); err != nil {
			return plugin.ErrorReply(msg, "failed_to_discover_plugins: "+err.Error())
		}
	}



	// Broadcast quiesce to all plugins to prepare for backup
	for _, t := range discovery.Targets {
		if t.ID == "project" || t.ID == "kernel" || t.ID == "command-manager" || t.ID == "iam" || t.ID == "events" || t.ID == "history" {
			continue
		}
		plugin.RouteMessage(AlloyMessage{
			Id:      "quiesce-" + fmt.Sprint(time.Now().UnixNano()),
			MsgType: "event", // Fire and forget for now
			Method:  "quiesce",
			Sender:  "project",
			Target:  Some(t.ID),
		})
	}

	// 2. Export state from each plugin
	states := make(map[string]json.RawMessage)
	for _, t := range discovery.Targets {
		if t.ID == "project" || t.ID == "kernel" || t.ID == "command-manager" || t.ID == "iam" || t.ID == "events" || t.ID == "history" {
			continue
		}

		plugin.Log("info", "Exporting state from plugin: "+t.ID)
		stateResp := plugin.Call(AlloyMessage{
			Id:      "state-export-" + fmt.Sprint(time.Now().UnixNano()),
			MsgType: "request",
			Method:  "state:export",
			Sender:  "project",
			Target:  Some(t.ID),
			Metadata: []AlloyTuple2StringStringT{{"alloy.no_audit", "true"}},
		})
		
		if len(stateResp.Payload) > 0 {
			states[t.ID] = stateResp.Payload
		}
	}

	// 3. Get event history
	plugin.Log("info", "Fetching event history...")
	historyResp := plugin.Call(AlloyMessage{
		Id:      "history-get-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "request",
		Method:  "history:get",
		Sender:  "project",
		Target:  Some("history"),
		Payload: []byte(`{"start": 0, "end": 0}`),
	})

	// 4. Create .tar.gz bundle
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add manifest
	manifest := map[string]interface{}{
		"version":    "1.0",
		"timestamp":  time.Now().Unix(),
		"projects":   projects,
		"workspaces": workspaces,
		"active_id":  activeWorkspaceID,
	}
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	if err := addFileToTar(tw, "manifest.json", manifestData); err != nil {
		return plugin.ErrorReply(msg, "failed_to_package_manifest: "+err.Error())
	}

	// Add history
	if len(historyResp.Payload) > 0 {
		if err := addFileToTar(tw, "history.json", historyResp.Payload); err != nil {
			return plugin.ErrorReply(msg, "failed_to_package_history: "+err.Error())
		}
	}

	// Add plugin states
	for id, state := range states {
		if err := addFileToTar(tw, "plugins/"+id+".json", state); err != nil {
			return plugin.ErrorReply(msg, "failed_to_package_plugin_state_"+id+": "+err.Error())
		}
	}

	tw.Close()
	gw.Close()

	// 5. Write archive to a file in the data directory
	filename := fmt.Sprintf("workspace-%s-%d.ark", activeWorkspaceID, time.Now().Unix())
	if err := os.WriteFile(filename, buf.Bytes(), 0644); err != nil {
		return plugin.ErrorReply(msg, "failed_to_write_archive: "+err.Error())
	}

	plugin.Log("info", "Workspace archived successfully: "+filename)

	// 6. Notify Librarian of new archive for indexing
	archiveMeta, _ := json.Marshal(map[string]string{
		"filename":   filename,
		"workspace":  activeWorkspaceID,
		"timestamp":  fmt.Sprintf("%d", time.Now().Unix()),
	})
	plugin.RouteMessage(AlloyMessage{
		Id:      "lib-archive-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "event",
		Method:  "librarian:index-archive",
		Sender:  "project",
		Target:  Some("librarian"),
		Payload: archiveMeta,
	})

	return plugin.Reply(msg, map[string]string{
		"filename": filename,
		"status":   "archived",
		"size":     fmt.Sprintf("%d bytes", buf.Len()),
	})
}

// handleListArchives returns a list of all .ark files in the data directory.
func handleListArchives(msg AlloyMessage) AlloyMessage {
	entries, err := os.ReadDir("/")
	if err != nil {
		return plugin.ErrorReply(msg, "failed_to_read_data_directory: "+err.Error())
	}

	archives := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ark") {
			archives = append(archives, entry.Name())
		}
	}

	return plugin.Reply(msg, map[string]interface{}{
		"archives": archives,
	})
}

// handleDeleteArchive deletes an archive from the data directory.
func handleDeleteArchive(msg AlloyMessage) AlloyMessage {
	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request: "+err.Error())
	}

	// Basic safety check: ensure the filename contains only an actual filename, not paths
	if strings.Contains(req.Filename, "/") || strings.Contains(req.Filename, "..") {
		return plugin.ErrorReply(msg, "invalid_filename")
	}

	if err := os.Remove(req.Filename); err != nil {
		return plugin.ErrorReply(msg, "failed_to_delete_archive: "+err.Error())
	}

	plugin.Log("info", "Deleted workspace archive: "+req.Filename)
	return plugin.Reply(msg, map[string]string{"status": "deleted"})
}

func addFileToTar(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// handleRestore restores a workspace from an .ark file
func handleRestore(msg AlloyMessage) AlloyMessage {
	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request: "+err.Error())
	}

	plugin.Log("info", "Starting workspace restoration from: "+req.Filename)

	// 1. Read and unpack the archive
	data, err := os.ReadFile(req.Filename)
	if err != nil {
		return plugin.ErrorReply(msg, "failed_to_read_archive: "+err.Error())
	}

	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return plugin.ErrorReply(msg, "invalid_archive_format (gzip): "+err.Error())
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	
	var manifest struct {
		Projects   map[string]*Project   `json:"projects"`
		Workspaces map[string]*Workspace `json:"workspaces"`
		ActiveID   string                `json:"active_id"`
	}
	pluginStates := make(map[string]json.RawMessage)
	var historyData json.RawMessage

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return plugin.ErrorReply(msg, "error_reading_tar: "+err.Error())
		}

		content, _ := io.ReadAll(tr)

		if header.Name == "manifest.json" {
			json.Unmarshal(content, &manifest)
		} else if header.Name == "history.json" {
			historyData = content
		} else if strings.HasPrefix(header.Name, "plugins/") {
			id := strings.TrimPrefix(header.Name, "plugins/")
			id = strings.TrimSuffix(id, ".json")
			pluginStates[id] = content
		}
	}

	// 2. Restore core metadata
	if manifest.Projects != nil {
		for id, p := range manifest.Projects {
			projects[id] = p
		}
		saveProjects()
	}
	if manifest.Workspaces != nil {
		for id, w := range manifest.Workspaces {
			workspaces[id] = w
		}
		saveWorkspaces()
	}
	if manifest.ActiveID != "" {
		activeWorkspaceID = manifest.ActiveID
		plugin.KVSet("active-workspace-id", []byte(activeWorkspaceID))
	}

	// 3. Restore plugin states
	for id, state := range pluginStates {
		plugin.Log("info", "Importing state into plugin: "+id)
		plugin.Call(AlloyMessage{
			Id:      "state-import-" + fmt.Sprint(time.Now().UnixNano()),
			MsgType: "request",
			Method:  "state:import",
			Sender:  "project",
			Target:  Some(id),
			Payload: state,
			Metadata: []AlloyTuple2StringStringT{{"alloy.no_audit", "true"}},
		})
	}

	// 4. Restore history
	if historyData != nil {
		plugin.Log("info", fmt.Sprintf("Restoring workspace history (%d bytes)...", len(historyData)))
		hResp := plugin.Call(AlloyMessage{
			Id:      "history-restore-" + fmt.Sprint(time.Now().UnixNano()),
			MsgType: "request",
			Method:  "history:restore",
			Sender:  "project",
			Target:  Some("history"),
			Payload: historyData,
		})
		plugin.Log("info", "History restoration response: "+string(hResp.Payload))
	}

	plugin.Log("info", "Workspace restored successfully from "+req.Filename)
	return plugin.Reply(msg, "restored")
}

