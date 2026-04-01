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
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Path      string            `json:"path"`
	TeamID    string            `json:"team_id,omitempty"`
	Layout    string            `json:"layout,omitempty"`
	ViewState string            `json:"view_state,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type ViewState struct {
	PaneID    string            `json:"pane_id,omitempty"`
	ScrollX   int               `json:"scroll_x,omitempty"`
	ScrollY   int               `json:"scroll_y,omitempty"`
	ActiveTab int               `json:"active_tab,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type WorkspaceConfig struct {
	DefaultMode string               `json:"default_mode"`
	Root        *LayoutNode          `json:"root,omitempty"`
	Views       map[string]ViewState `json:"views,omitempty"` // Map of PaneID -> ViewState
}

type LayoutNode struct {
	ID        string       `json:"id,omitempty"`        // Unique identifier
	Type      string       `json:"type"`                // "split" or "pane"
	Direction string       `json:"direction,omitempty"` // "horizontal" or "vertical"
	Weight    float64      `json:"weight,omitempty"`    // Ratio (e.g., 0.5)
	Children  []LayoutNode `json:"children,omitempty"`  // Nested nodes
	PluginID  string       `json:"plugin_id,omitempty"` // For "pane" type
	Mode      string       `json:"mode,omitempty"`      // For "pane" (dashboard, chat, etc.)
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
