//go:build wasip1 || wasm

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/james-nesbitt/alloy/pkg/wasm/guest"
)

func main() {
	plugin := guest.NewPlugin("filesystem-provider")

	// Advertise the discovery capability
	plugin.RegisterMethod("base:provider:path", "Filesystem-based Base discovery provider", nil).WithAdvertisement()

	// Implement discovery: base:discover
	plugin.RegisterMethod("base:discover", "Discover Base configuration via filesystem scan", func(msg guest.AlloyMessage) *guest.AlloyMessage {
		var req struct {
			Root     string         `json:"root"`
			BaseID   string         `json:"base_id"`
			Metadata map[string]any `json:"metadata"`
		}

		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return errorReply(msg, err)
		}

		root := req.Root
		if root == "" {
			root = "/work"
		}

		alloyDir := filepath.Join(root, ".alloy")
		if _, err := os.Stat(alloyDir); os.IsNotExist(err) {
			return errorReply(msg, fmt.Errorf(".alloy directory not found in %s", root))
		}

		manifest := &guest.BaseManifest{
			Plugins: make(map[string]json.RawMessage),
		}

		// Look for Phase 13 structure: .alloy/plugins/[[id]]/config.json
		pluginsDir := filepath.Join(alloyDir, "plugins")
		if entries, err := os.ReadDir(pluginsDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				pluginID := entry.Name()
				configPath := filepath.Join(pluginsDir, pluginID, "config.json")
				if content, err := os.ReadFile(configPath); err == nil {
					manifest.Plugins[pluginID] = content
				}
			}
		} else {
			// Fallback (Phase 7/11): .alloy/*.json
			files, err := os.ReadDir(alloyDir)
			if err == nil {
				for _, f := range files {
					if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
						continue
					}
					pluginID := strings.TrimSuffix(f.Name(), ".json")
					if content, err := os.ReadFile(filepath.Join(alloyDir, f.Name())); err == nil {
						manifest.Plugins[pluginID] = content
					}
				}
			}
		}

		payload, _ := json.Marshal(manifest)
		return &guest.AlloyMessage{
			Id:      msg.Id + "-resp",
			Method:  msg.Method,
			Payload: payload,
			Target:  msg.Target,
		}
	})

	plugin.Log(guest.LogLevelInfo, "Filesystem Base Provider initialized")
	plugin.Serve()
}

func errorReply(msg guest.AlloyMessage, err error) *guest.AlloyMessage {
	return &guest.AlloyMessage{
		Id:      msg.Id + "-resp",
		Method:  msg.Method,
		Payload: []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())),
		Target:  msg.Target,
	}
}
