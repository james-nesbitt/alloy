package kernel

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
		{Method: "subscribe", Description: "Subscribe to an event topic (Legacy)"},
		{Method: "publish", Description: "Publish an event to a topic (Legacy)"},
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

		e.Publish(ctx, req.Topic, msg.Sender, req.Data)

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

// Publish distributes an event to all subscribers.
func (e *EventManager) Publish(ctx context.Context, topic string, sender string, data json.RawMessage) {
	e.mu.RLock()
	subs := e.subscribers[topic]
	e.mu.RUnlock()

	e.logger.Info("publishing event", "topic", topic, "subscribers", len(subs), "sender", sender)

	if e.route != nil {
		for _, sub := range subs {
			go e.route(ctx, api.Message{
				ID:        "evt-" + time.Now().Format("150405.000"),
				Type:      api.TypeEvent,
				Sender:    e.ID(),
				Target:    sub,
				Method:    topic,
				Payload:   data,
				Timestamp: time.Now().Unix(),
			})
		}
	}
}

func (e *EventManager) Shutdown(ctx context.Context) error {
	return nil
}
