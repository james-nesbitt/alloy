package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

type subscriber struct {
	ID    string
	Actor string
}

type patternSubscriber struct {
	Pattern *regexp.Regexp
	ID      string
	Actor   string
}

// EventManager handles pub/sub for the Alloy ecosystem.
type EventManager struct {
	mu                 sync.RWMutex
	subscribers        map[string][]subscriber // topic -> subscribers
	patternSubscribers []patternSubscriber     // Registered patterns
	logger             *slog.Logger
	route              func(context.Context, api.Message)
	iam                *IdentityManager
}

func NewEventManager(logger *slog.Logger, iam *IdentityManager) *EventManager {
	return &EventManager{
		subscribers:        make(map[string][]subscriber),
		patternSubscribers: make([]patternSubscriber, 0),
		logger:             logger,
		iam:                iam,
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
			Topic   string `json:"topic"`
			Pattern string `json:"pattern"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		actor := msg.Actor
		if actor == "" {
			actor = msg.Sender
		}

		e.mu.Lock()
		if req.Pattern != "" {
			re, err := regexp.Compile(req.Pattern)
			if err != nil {
				e.mu.Unlock()
				return api.Message{}, fmt.Errorf("invalid pattern: %w", err)
			}
			e.patternSubscribers = append(e.patternSubscribers, patternSubscriber{
				Pattern: re,
				ID:      msg.Sender,
				Actor:   actor,
			})
			e.logger.Debug("plugin subscribed to pattern", "plugin", msg.Sender, "pattern", req.Pattern)
		}

		if req.Topic != "" {
			e.subscribers[req.Topic] = append(e.subscribers[req.Topic], subscriber{
				ID:    msg.Sender,
				Actor: actor,
			})
			e.logger.Debug("plugin subscribed to topic", "plugin", msg.Sender, "topic", req.Topic)
		}
		e.mu.Unlock()

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
			Scope string          `json:"scope,omitempty"`
			Data  json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		e.PublishScoped(ctx, req.Topic, msg.Sender, req.Data, req.Scope)

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
	e.PublishScoped(ctx, topic, sender, data, "")
}

// PublishScoped distributes an event with a namespace scope (context).
func (e *EventManager) PublishScoped(ctx context.Context, topic string, sender string, data json.RawMessage, scope string) {
	e.mu.RLock()
	var subs []subscriber
	if s, ok := e.subscribers[topic]; ok {
		subs = append(subs, s...)
	}

	for _, ps := range e.patternSubscribers {
		if ps.Pattern.MatchString(topic) {
			subs = append(subs, subscriber{ID: ps.ID, Actor: ps.Actor})
		}
	}
	e.mu.RUnlock()

	// Deduplicate subscribers
	seen := make(map[string]bool)
	var finalSubs []subscriber
	for _, sub := range subs {
		if !seen[sub.ID] {
			finalSubs = append(finalSubs, sub)
			seen[sub.ID] = true
		}
	}

	e.logger.Info("publishing event", "topic", topic, "subscribers", len(finalSubs), "sender", sender, "scope", scope)

	if e.route != nil {
		for _, sub := range finalSubs {
			// Pre-flight Redaction Logic
			if scope != "" && e.iam != nil {
				// Check if this subscriber actor can receive this topic in this scope
				if !e.iam.AuthorizeWithContext(sub.Actor, "events", "subscribe", scope) {
					e.logger.Debug("skipping event for unauthorized subscriber", "topic", topic, "sub", sub.ID, "actor", sub.Actor, "scope", scope)
					continue
				}
			}

			go e.route(ctx, api.Message{
				ID:        "evt-" + time.Now().Format("150405.000"),
				Type:      api.TypeEvent,
				Sender:    e.ID(),
				Target:    sub.ID,
				Method:    topic,
				Payload:   data,
				Timestamp: time.Now().Unix(),
				Metadata: map[string]any{
					"scope": scope,
				},
			})
		}
	}
}

// HasSubscribers checks if there are any subscribers for a given topic.
func (e *EventManager) HasSubscribers(topic string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.subscribers[topic]) > 0
}

func (e *EventManager) Shutdown(ctx context.Context) error {
	return nil
}
