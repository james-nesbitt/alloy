package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

// handleKernelMessage handles requests targeted at the kernel itself.
func (k *Kernel) handleKernelMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "bootstrap-path":
		var path string
		if err := json.Unmarshal(msg.Payload, &path); err != nil {
			path = string(msg.Payload)
		}
		if path == "" {
			return api.Message{}, fmt.Errorf("missing path in payload")
		}

		err := k.BootstrapPath(ctx, path)
		if err != nil {
			k.logger.Error("failed to bootstrap path", "path", path, "error", err)
			return api.Message{}, err
		}

		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    "kernel",
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"ok"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	default:
		return api.Message{}, fmt.Errorf("unknown kernel method: %s", msg.Method)
	}
}

// BootstrapPath performs discovery and activation on a given base path.
func (k *Kernel) BootstrapPath(ctx context.Context, baseCtx string) error {
	k.logger.Info("bootstrapping path", "path", baseCtx)
	alloyDir := filepath.Join(baseCtx, ".alloy")
	if _, err := os.Stat(alloyDir); os.IsNotExist(err) {
		return fmt.Errorf(".alloy directory not found in %s", baseCtx)
	}

	// 1. Stable Socket Creation
	sockAddr := "unix://" + filepath.Join(alloyDir, "alloy.sock")
	k.mu.RLock()
	onAddListener := k.onAddListener
	k.mu.RUnlock()

	if onAddListener != nil {
		k.logger.Info("registering project socket", "path", sockAddr)
		if err := onAddListener(sockAddr); err != nil {
			k.logger.Warn("failed to register project socket", "path", sockAddr, "error", err)
		}
	}

	// 2. Discovery & Activation
	files, err := os.ReadDir(alloyDir)
	if err != nil {
		return err
	}

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}

		pluginID := strings.TrimSuffix(f.Name(), ".json")
		configPath := filepath.Join(alloyDir, f.Name())

		content, err := os.ReadFile(configPath)
		if err != nil {
			k.logger.Error("failed to read plugin config", "id", pluginID, "path", configPath, "error", err)
			continue
		}

		// Routes content as a generic configuration message
		k.RouteMessage(ctx, api.Message{
			ID:        fmt.Sprintf("bootstrap-%s-%d", pluginID, time.Now().UnixNano()),
			Type:      api.TypeRequest,
			Sender:    "kernel",
			Target:    pluginID,
			Method:    "config:update",
			Payload:   content,
			Timestamp: time.Now().Unix(),
			Metadata: map[string]any{
				"project_path": baseCtx,
			},
		})
	}

	return nil
}
