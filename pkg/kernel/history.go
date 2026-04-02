package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage/history"
)

type HistoryManager struct {
	logger *slog.Logger
	store  *history.Store
}

func NewHistoryManager(logger *slog.Logger, store *history.Store) *HistoryManager {
	return &HistoryManager{
		logger: logger,
		store:  store,
	}
}

func (h *HistoryManager) ID() string { return "history" }

func (h *HistoryManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "history:get", Description: "Get workspace event history"},
		{Method: "history:replay", Description: "Replay a range of events"},
	}
}

func (h *HistoryManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "history:get":
		var req struct {
			Start uint64 `json:"start"`
			End   uint64 `json:"end"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		events, err := h.store.GetRange(req.Start, req.End)
		if err != nil {
			return api.Message{}, err
		}

		payload, _ := json.Marshal(events)
		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Method:  msg.Method,
			Sender:  "history",
			Payload: payload,
		}, nil

	case "history:replay":
		// TODO: Implement replay logic (routing events back through the kernel with a 'replay' flag)
		return api.Message{ID: msg.ID + "-resp", Type: api.TypeResponse, Method: msg.Method, Payload: []byte(`{"status":"unimplemented"}`)}, nil
	}

	return api.Message{}, fmt.Errorf("method_not_found")
}

func (h *HistoryManager) Shutdown(ctx context.Context) error {
	return nil // Store is managed by Kernel
}
