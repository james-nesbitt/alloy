package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
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

		// Phase 13: Refactor to Base-led activation via Capability
		activationPayload, _ := json.Marshal(map[string]any{
			"base_id":    "default-" + filepath.Base(path),
			"capability": "base:provider:path",
			"metadata": map[string]any{
				"project_path": path,
			},
		})
		k.RouteMessage(ctx, api.Message{
			ID:      "bootstrap-base-" + fmt.Sprint(time.Now().UnixNano()),
			Type:    api.TypeRequest,
			Sender:  "kernel",
			Target:  "base-manager",
			Method:  "activate",
			Payload: activationPayload,
		})

		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    "kernel",
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"bootstrapping"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	default:
		return api.Message{}, fmt.Errorf("unknown kernel method: %s", msg.Method)
	}
}

// BootstrapPath is now a wrapper that emulates the legacy behavior using Base-led activation.
func (k *Kernel) BootstrapPath(ctx context.Context, baseCtx string) error {
	payload, _ := json.Marshal(map[string]any{
		"base_id":    "legacy-" + filepath.Base(baseCtx),
		"capability": "base:provider:path",
		"metadata": map[string]any{
			"project_path": baseCtx,
		},
	})
	k.RouteMessage(ctx, api.Message{
		ID:      "legacy-bootstrap-" + fmt.Sprint(time.Now().UnixNano()),
		Type:    api.TypeRequest,
		Sender:  "kernel",
		Target:  "base-manager",
		Method:  "activate",
		Payload: payload,
	})
	return nil
}
