package frontend

import "github.com/james-nesbitt/alloy/api"

type DashboardTile struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	ContentType string   `json:"content_type"` // "text", "markdown", "json", "ascii"
	Content     []string `json:"content"`      // Backward compatibility or for text
	RawContent  []byte   `json:"raw_content,omitempty"`
	Status      string   `json:"status"`
	Actions     []string `json:"actions"`
	Timestamp   int64    `json:"timestamp"`
	RefreshMS   uint32   `json:"refresh_ms"`
}

type WorkspaceConfig = api.WorkspaceConfig
type Project = api.Project
type Workspace = api.Workspace
type Presence = api.Presence
