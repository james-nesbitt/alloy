package native

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

// EventManager handles pub/sub for the Alloy ecosystem.
type EventManager struct {
	mu          sync.RWMutex
	subscribers map[string][]string // topic -> subscriber plugin/frontend IDs
	logger      *slog.Logger
	route       func(context.Context, api.Message)
}

func NewEventManager(logger *slog.Logger) *EventManager {
	return &EventManager{
		subscribers: make(map[string][]string),
		logger:      logger,
	}
}

func (e *EventManager) SetRouter(r func(context.Context, api.Message)) {
	e.route = r
}

func (e *EventManager) ID() string { return "events" }

func (e *EventManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "events:subscribe", Description: "Subscribe to an event topic"},
		{Method: "events:publish", Description: "Publish an event to a topic"},
		// Backward compatibility
		{Method: "subscribe", Description: "Subscribe to an event topic"},
		{Method: "publish", Description: "Publish an event to a topic"},
	}
}

func (e *EventManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "subscribe", "events:subscribe":
		var req struct {
			Topic string `json:"topic"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}
		e.mu.Lock()
		e.subscribers[req.Topic] = append(e.subscribers[req.Topic], msg.Sender)
		e.mu.Unlock()
		e.logger.Debug("plugin subscribed to topic", "plugin", msg.Sender, "topic", req.Topic)
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    e.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"subscribed"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	case "publish", "events:publish":
		var req struct {
			Topic string          `json:"topic"`
			Data  json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		e.mu.RLock()
		subs := e.subscribers[req.Topic]
		e.mu.RUnlock()

		e.logger.Info("publishing event", "topic", req.Topic, "subscribers", len(subs), "sender", msg.Sender)

		if e.route != nil {
			for _, sub := range subs {
				go e.route(ctx, api.Message{
					ID:        "evt-" + time.Now().Format("150405.000"),
					Type:      api.TypeEvent,
					Sender:    e.ID(),
					Target:    sub,
					Method:    req.Topic,
					Payload:   req.Data,
					Timestamp: time.Now().Unix(),
				})
			}
		}

		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    e.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"published"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	}

	return api.Message{}, nil
}

func (e *EventManager) Shutdown(ctx context.Context) error {
	return nil
}
