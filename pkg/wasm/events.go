package wasm

import (
	"context"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

// EventManager handles pub/sub for the Alloy ecosystem.
type EventManager struct {
	mu          sync.RWMutex
	subscribers map[string][]chan<- api.Message
}

func NewEventManager() *EventManager {
	return &EventManager{
		subscribers: make(map[string][]chan<- api.Message),
	}
}

func (e *EventManager) ID() string { return "plugin-events" }

func (e *EventManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "subscribe", Description: "Subscribe to an event topic"},
		{Method: "publish", Description: "Publish an event to a topic"},
	}
}

func (e *EventManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "subscribe":
		// Subscription logic would usually return a stream or register a callback.
		// For now, this is a simplified stub.
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    e.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"subscribed"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	case "publish":
		// Publish an event of TypeEvent to all subscribers
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
