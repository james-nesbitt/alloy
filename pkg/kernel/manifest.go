package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/james-nesbitt/alloy/api"
)

// ProjectManifest defines the structure of the alloy-project.json file.
type ProjectManifest struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Version     string              `json:"version,omitempty"`
	Plugins     []PluginConfig      `json:"plugins"`
	Layout      api.WorkspaceConfig `json:"layout,omitempty"`
}

// PluginConfig describes a plugin in the manifest.
type PluginConfig struct {
	ID       string             `json:"id"`
	Path     string             `json:"path"`
	LoadTime api.PluginLoadTime `json:"load"` // "boot" or "lazy"
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

// ApplyManifest auto-boots any plugins marked for "boot" and registers the project in the kernel.
func (k *Kernel) ApplyManifest(ctx context.Context, manifest *ProjectManifest) error {
	// 1. Register the project identity in the kernel
	project := api.Project{
		ID:          manifest.Name, // Use name as ID for now or generate a hash
		Name:        manifest.Name,
		Description: manifest.Description,
		Layout:      manifest.Layout,
	}

	// For now, we reuse the workspace registration since they're functionally similar handles
	k.RegisterWorkspace(api.Workspace{
		ID:     project.ID,
		Name:   project.Name,
		Layout: string(manifest.Layout.DefaultMode), // simple mapping
	})

	k.logger.Info("applying manifest", "project", manifest.Name, "plugins", len(manifest.Plugins))

	// 2. Load plugins
	for _, pc := range manifest.Plugins {
		// Resolve relative paths if necessary
		pluginPath := pc.Path
		if !filepath.IsAbs(pluginPath) {
			// Assume relative to current working directory or binary dir
			// Let's use the helper to find it
			pluginPath = ResolvePluginPath("alloy-project.json", pc.Path)
		}

		k.logger.Debug("registering plugin from manifest", "id", pc.ID, "load", pc.LoadTime)

		if pc.LoadTime == api.LoadTimeBoot {
			wasmBytes, err := os.ReadFile(pluginPath)
			if err != nil {
				k.logger.Error("failed to read plugin WASM from manifest", "id", pc.ID, "path", pluginPath, "error", err)
				continue
			}
			if err := k.RegisterWASMPluginAtScale(pc.ID, wasmBytes, 128, 1000, nil); err != nil {
				k.logger.Error("failed to boot plugin from manifest", "id", pc.ID, "error", err)
			}
		} else {
			// Lazy loading
			k.RegisterMetadata(api.PluginMetadata{
				ID:           pc.ID,
				LoadTime:     api.LoadTimeLazy,
				Capabilities: nil,
			}, &wasmLoader{
				k:            k,
				pluginID:     pc.ID,
				path:         pluginPath,
				logger:       k.logger,
				maxMemoryMB:  128,
				msgPerSecond: 1000,
			})
		}
	}

	return nil
}
