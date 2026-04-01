package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

// IntentBroker manages goal-oriented routing (Phase 10)
type IntentBroker struct {
	logger *slog.Logger
	mu     sync.RWMutex
	// intent name -> list of plugin IDs
	providers map[string][]string
	router    func(context.Context, api.Message)
}

// NewIntentBroker creates a new IntentBroker
func NewIntentBroker(logger *slog.Logger, router func(context.Context, api.Message)) *IntentBroker {
	return &IntentBroker{
		logger:    logger,
		providers: make(map[string][]string),
		router:    router,
	}
}

// Register registers a plugin for a list of intents
func (b *IntentBroker) Register(pluginID string, intents []string) {
	if len(intents) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, intent := range intents {
		exists := false
		for _, p := range b.providers[intent] {
			if p == pluginID {
				exists = true
				break
			}
		}
		if !exists {
			b.providers[intent] = append(b.providers[intent], pluginID)
			b.logger.Debug("registered intent provider", "intent", intent, "plugin", pluginID)
		}
	}
}

// Unregister unregisters a plugin from all intents
func (b *IntentBroker) Unregister(pluginID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for intent, plugins := range b.providers {
		for i, p := range plugins {
			if p == pluginID {
				b.providers[intent] = append(plugins[:i], plugins[i+1:]...)
				break
			}
		}
	}
}

// Dispatch routes an intent to a provider
func (b *IntentBroker) Dispatch(ctx context.Context, intent api.Intent) error {
	b.mu.RLock()
	plugins, ok := b.providers[intent.Name]
	b.mu.RUnlock()

	if !ok || len(plugins) == 0 {
		b.logger.Warn("no provider for intent", "intent", intent.Name)
		return fmt.Errorf("no provider for intent: %s", intent.Name)
	}

	// For Phase 10, picked the first available provider.
	// Optimization: This could use priority, load-balancing, or active context in the future.
	target := plugins[0]

	msg := api.Message{
		ID:        intent.ID,
		Type:      api.TypeRequest,
		Sender:    intent.Sender,
		Target:    target,
		Method:    intent.Name,
		Payload:   intent.Payload,
		Timestamp: time.Now().Unix(),
	}

	if intent.ContextID != "" {
		if msg.Metadata == nil {
			msg.Metadata = make(map[string]any)
		}
		msg.Metadata["context_id"] = intent.ContextID
	}

	b.logger.Info("dispatching intent", "intent", intent.Name, "target", target)
	b.router(ctx, msg)
	return nil
}
