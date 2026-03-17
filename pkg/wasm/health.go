package wasm

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

// HealthManager monitors system status.
type HealthManager struct{}

func NewHealthManager() *HealthManager {
	return &HealthManager{}
}

func (h *HealthManager) ID() string { return "plugin-health" }

func (h *HealthManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "status", Description: "Get the overall health of the system"},
	}
}

func (h *HealthManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "status":
		status := map[string]any{
			"status": "healthy",
			"uptime": "not implemented",
		}
		payload, _ := json.Marshal(status)
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    h.ID(),
			Target:    msg.Sender,
			Payload:   payload,
			Timestamp: time.Now().Unix(),
		}, nil
	}
	return api.Message{}, nil
}

func (h *HealthManager) Shutdown(ctx context.Context) error {
	return nil
}
