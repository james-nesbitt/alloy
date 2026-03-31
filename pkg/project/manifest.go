package project

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/james-nesbitt/alloy/api"
)

// WorkspaceConfig defines the visual and operational layout for a project.
type WorkspaceConfig struct {
	DefaultMode string `json:"default_mode"`
	Layout      []struct {
		Type     string  `json:"type"` // "dashboard", "chat", "editor", "status"
		WidthPct float64 `json:"width_pct"`
	} `json:"layout"`
}

// ProjectManifest defines the structure of the alloy-project.json file.
type ProjectManifest struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Version     string          `json:"version,omitempty"`
	Plugins     []PluginConfig  `json:"plugins"`
	Layout      WorkspaceConfig `json:"layout,omitempty"`
	Security    *SecurityConfig `json:"security,omitempty"`
}

// PluginConfig describes a plugin in the manifest.
type PluginConfig struct {
	ID       string             `json:"id"`
	Path     string             `json:"path"`
	LoadTime api.PluginLoadTime `json:"load"` // "boot" or "lazy"
}

// SecurityConfig defines roles and assignments for the project.
type SecurityConfig struct {
	Roles       map[string][]string `json:"roles"`       // roleName -> []capabilities
	Assignments map[string]string   `json:"assignments"` // actorFingerprint -> roleName
}

// LoadManifest reads and parses the project manifest from the given path.
func LoadManifest(path string) (*ProjectManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest ProjectManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &manifest, nil
}
