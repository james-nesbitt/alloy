package native

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
)

type Buffer struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Type      string `json:"type"` // e.g., "text", "markdown", "go"
	Version   int    `json:"version"`
	UpdatedAt int64  `json:"updated_at"`
}

type BufferManager struct {
	logger *slog.Logger
	state  storage.StateStore
	mu     sync.RWMutex
	router func(ctx context.Context, msg api.Message)
	buffers map[string]*Buffer
}

func NewBufferManager(logger *slog.Logger, state storage.StateStore) *BufferManager {
	return &BufferManager{
		logger:  logger,
		state:   state,
		buffers: make(map[string]*Buffer),
	}
}

func (b *BufferManager) SetRouter(router func(ctx context.Context, msg api.Message)) {
	b.router = router
}

func (b *BufferManager) ID() string { return "plugin-buffer-manager" }

func (b *BufferManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "open", Description: "Open or create a buffer"},
		{Method: "get", Description: "Get buffer content"},
		{Method: "update", Description: "Update buffer content"},
		{Method: "list", Description: "List all open buffers"},
		{Method: "close", Description: "Close an open buffer"},
	}
}

func (b *BufferManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "open":
		var req struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		b.mu.Lock()
		defer b.mu.Unlock()

		buf, ok := b.buffers[req.ID]
		if !ok {
			// Try to load from state
			data, err := b.state.Get(b.ID(), req.ID)
			if err == nil && data != nil {
				if err := json.Unmarshal(data, &buf); err != nil {
					b.logger.Error("failed to unmarshal buffer from state", "id", req.ID, "error", err)
				}
			}

			if buf == nil {
				buf = &Buffer{
					ID:        req.ID,
					Type:      req.Type,
					Version:   0,
					UpdatedAt: time.Now().Unix(),
				}
			}
			b.buffers[req.ID] = buf
		}

		payload, _ := json.Marshal(buf)
		return b.response(msg, payload), nil

	case "get":
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		b.mu.RLock()
		buf, ok := b.buffers[req.ID]
		b.mu.RUnlock()

		if !ok {
			return api.Message{}, fmt.Errorf("buffer not found: %s", req.ID)
		}

		payload, _ := json.Marshal(buf)
		return b.response(msg, payload), nil

	case "update":
		var req struct {
			ID      string `json:"id"`
			Content string `json:"content"`
			Version int    `json:"version"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		b.mu.Lock()
		buf, ok := b.buffers[req.ID]
		if !ok {
			b.mu.Unlock()
			return api.Message{}, fmt.Errorf("buffer not found: %s", req.ID)
		}

		// Simple conflict resolution: version must be higher or equal (naive)
		if req.Version < buf.Version {
			b.mu.Unlock()
			return api.Message{}, fmt.Errorf("version conflict: local=%d, received=%d", buf.Version, req.Version)
		}

		buf.Content = req.Content
		buf.Version = req.Version + 1
		buf.UpdatedAt = time.Now().Unix()
		b.mu.Unlock()

		// Persist
		data, _ := json.Marshal(buf)
		_ = b.state.Set(b.ID(), buf.ID, data)

		// Notify via Event Bus
		if b.router != nil {
			eventPayload, _ := json.Marshal(map[string]any{
				"topic": "buffer:updated",
				"data":  buf,
			})
			b.router(ctx, api.Message{
				ID:        fmt.Sprintf("buf-evt-%d", time.Now().UnixNano()),
				Type:      api.TypeEvent,
				Sender:    b.ID(),
				Target:    "plugin-events",
				Method:    "publish",
				Payload:   eventPayload,
				Timestamp: time.Now().Unix(),
			})
		}

		return b.response(msg, data), nil

	case "list":
		b.mu.RLock()
		var list []string
		for id := range b.buffers {
			list = append(list, id)
		}
		b.mu.RUnlock()

		payload, _ := json.Marshal(list)
		return b.response(msg, payload), nil

	case "close":
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		b.mu.Lock()
		delete(b.buffers, req.ID)
		b.mu.Unlock()

		return b.response(msg, []byte(`{"status":"closed"}`)), nil
	}

	return api.Message{}, fmt.Errorf("unknown method: %s", msg.Method)
}

func (b *BufferManager) response(msg api.Message, payload []byte) api.Message {
	return api.Message{
		ID:        msg.ID + "-resp",
		Type:      api.TypeResponse,
		Sender:    b.ID(),
		Target:    msg.Sender,
		Method:    msg.Method,
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}
}

func (b *BufferManager) Shutdown(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Final persist of all open buffers
	for _, buf := range b.buffers {
		data, _ := json.Marshal(buf)
		_ = b.state.Set(b.ID(), buf.ID, data)
	}
	return nil
}
