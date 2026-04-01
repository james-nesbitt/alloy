package project

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/james-nesbitt/alloy/api"
)

// WorkspaceConfig defines the visual and operational layout for a project.
type WorkspaceConfig struct {
	DefaultMode string      `json:"default_mode"`
	Root        *LayoutNode `json:"root,omitempty"`
}

// LayoutNode defines a node in a recursive layout tree.
type LayoutNode struct {
	Type      string       `json:"type"`                // "split" or "pane"
	Direction string       `json:"direction,omitempty"` // "horizontal" or "vertical"
	Weight    float64      `json:"weight,omitempty"`    // Ratio (e.g., 0.5)
	Children  []LayoutNode `json:"children,omitempty"`  // Nested nodes
	PluginID  string       `json:"plugin_id,omitempty"` // For "pane" type
	Mode      string       `json:"mode,omitempty"`      // For "pane" (dashboard, chat, etc.)
}

// ProjectManifest defines the structure of the alloy-project.json file.
type ProjectManifest struct {
	ID          string          `json:"id,omitempty"`
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

// UserConfig defines global user preferences and side-car plugins.
type UserConfig struct {
	Sidecars []PluginConfig `json:"sidecars"`
	Theme    string         `json:"theme,omitempty"`
	Identity struct {
		DefaultRole string `json:"default_role,omitempty"`
	} `json:"identity,omitempty"`
}

// LoadUserConfig reads and parses the user config from the given path.
// If the file doesn't exist, it returns an empty config.
func LoadUserConfig(path string) (*UserConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &UserConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read user config: %w", err)
	}

	var config UserConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse user config: %w", err)
	}

	return &config, nil
}

// SecurityConfig defines roles and assignments for the project.
type SecurityConfig struct {
	Roles       map[string][]string `json:"roles"`       // roleName -> []capabilities
	Assignments map[string]string   `json:"assignments"` // actorFingerprint -> roleName
}

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
