package project

import (
	"github.com/james-nesbitt/alloy/api"
)

// ComposedWorkspace represents the synthesized view of a workspace.
type ComposedWorkspace struct {
	Name        string               `json:"name"`
	ContextID   string               `json:"context_id"`
	Plugins     []api.PluginMetadata `json:"plugins"`
	Layout      WorkspaceConfig      `json:"layout"`
	UserConfigs UserConfig           `json:"user_configs"`
}

// CompositionEngine handles merging project and user configurations.
type CompositionEngine struct {
	// ...
}

// Compose merges a project manifest and user configuration into a unified workspace view.
func Compose(manifest *ProjectManifest, userConfig *UserConfig, activePlugins []api.PluginMetadata) *ComposedWorkspace {
	contextID := manifest.ID
	if contextID == "" {
		contextID = manifest.Name
	}

	composed := &ComposedWorkspace{
		Name:        manifest.Name,
		ContextID:   contextID,
		Layout:      manifest.Layout,
		UserConfigs: *userConfig,
	}

	// Index active plugins by ID
	pluginMap := make(map[string]api.PluginMetadata)
	for _, p := range activePlugins {
		pluginMap[p.ID] = p
	}

	// Select relevant plugins
	// 1. All user side-cars
	for _, sc := range userConfig.Sidecars {
		if p, ok := pluginMap[sc.ID]; ok {
			composed.Plugins = append(composed.Plugins, p)
		}
	}

	// 2. All project plugins (if they don't conflict or are preferred)
	for _, pp := range manifest.Plugins {
		// Check if already added via side-car
		exists := false
		for _, p := range composed.Plugins {
			if p.ID == pp.ID {
				exists = true
				break
			}
		}
		if !exists {
			if p, ok := pluginMap[pp.ID]; ok {
				composed.Plugins = append(composed.Plugins, p)
			}
		}
	}

	return composed
}
