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

type WorkspaceConfig struct {
	DefaultMode string `json:"default_mode"`
	Dashboard   struct {
		Tiles []struct {
			Plugin string `json:"plugin"`
			Weight int    `json:"weight"`
		} `json:"tiles"`
	} `json:"dashboard"`
}

type Project struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Layout      WorkspaceConfig `json:"layout,omitempty"`
}

type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type Presence struct {
	User      string `json:"user"`
	Status    string `json:"status"`
	LastSeen  int64  `json:"last_seen"`
	Client    string `json:"client"`
	ProjectID string `json:"project_id,omitempty"`
}
