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
	logger   *slog.Logger
	store    *history.Store
	replayer func(ctx context.Context, start, end uint64) error
}

func NewHistoryManager(logger *slog.Logger, store *history.Store, replayer func(ctx context.Context, start, end uint64) error) *HistoryManager {
	return &HistoryManager{
		logger:   logger,
		store:    store,
		replayer: replayer,
	}
}

func (h *HistoryManager) ID() string { return "history" }

func (h *HistoryManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "history:get", Description: "Get workspace event history"},
		{Method: "history:replay", Description: "Replay a range of events"},
		{Method: "history:rollback", Description: "Rollback workspace state to a specific history index"},
		{Method: "history:restore", Description: "Restore complete history from a set of archived events"},
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
		var req struct {
			Start uint64 `json:"start"`
			End   uint64 `json:"end"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		if h.replayer != nil {
			if err := h.replayer(ctx, req.Start, req.End); err != nil {
				return api.Message{}, err
			}
		}

		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Method:  msg.Method,
			Sender:  "history",
			Payload: []byte(`{"status":"ok"}`),
		}, nil

	case "history:rollback":
		var req struct {
			Index uint64 `json:"index"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		if err := h.store.TruncateTo(req.Index); err != nil {
			return api.Message{}, err
		}

		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Method:  msg.Method,
			Sender:  "history",
			Payload: []byte(`{"status":"ok"}`),
		}, nil

	case "history:restore":
		var events []history.Event
		if err := json.Unmarshal(msg.Payload, &events); err != nil {
			return api.Message{}, err
		}

		if err := h.store.Restore(events); err != nil {
			return api.Message{}, err
		}

		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Method:  msg.Method,
			Sender:  "history",
			Payload: []byte(`{"status":"ok"}`),
		}, nil

	default:
		return api.Message{}, fmt.Errorf("method_not_found")
	}
}

func (h *HistoryManager) Shutdown(ctx context.Context) error {
	return nil // Store is managed by Kernel
}
