package frontend

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

type Workspace struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Path     string            `json:"path"`
	TeamID   string            `json:"team_id,omitempty"`
	Layout   string            `json:"layout,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type WorkspaceConfig struct {
	DefaultMode string `json:"default_mode"`
	Layout      []struct {
		Type     string  `json:"type"` // "dashboard", "chat", "editor", "status"
		WidthPct float64 `json:"width_pct"`
	} `json:"layout"`
}

type Project struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Layout      WorkspaceConfig `json:"layout,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
}

type Presence struct {
	User      string `json:"user"`
	Status    string `json:"status"`
	LastSeen  int64  `json:"last_seen"`
	Client    string `json:"client"`
	ProjectID string `json:"project_id,omitempty"`
}
