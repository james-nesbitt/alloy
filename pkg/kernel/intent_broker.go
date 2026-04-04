package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

// IntentBroker manages goal-oriented routing (Phase 10)
type IntentBroker struct {
	logger *slog.Logger
	mu     sync.RWMutex
	// intent name -> list of plugin IDs
	providers        map[string][]string
	router           func(ctx context.Context, msg api.Message)
	librarianQuerier func(ctx context.Context, query string) (string, error) // For semantic context injection (Phase 11)
}

// NewIntentBroker creates a new IntentBroker. If querier is non-nil, it will be used for automated context injection.
func NewIntentBroker(logger *slog.Logger, router func(ctx context.Context, msg api.Message), querier func(ctx context.Context, query string) (string, error)) *IntentBroker {
	return &IntentBroker{
		logger:           logger,
		providers:        make(map[string][]string),
		router:           router,
		librarianQuerier: querier,
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

	var target string
	if !ok || len(plugins) == 0 {
		// Phase 12: Broaden proactive interventions - allow 'intent:propose' and 'intent:suggest'
		// to broadcast if no explicit provider is found (typically reaches human frontends).
		if intent.Name == "intent:propose" || intent.Name == "intent:suggest" {
			b.logger.Debug("routing proactive intent to broadcast (no explicit provider)", "intent", intent.Name)
			target = "*"
		} else {
			b.logger.Warn("no provider for intent", "intent", intent.Name)
			return fmt.Errorf("no provider for intent: %s", intent.Name)
		}
	} else {
		// For Phase 10, picked the first available provider.
		// Optimization: This could use priority, load-balancing, or active context in the future.
		target = plugins[0]
	}

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

	// Phase 11: Semantic Context Injection - for AI-bound intents
	if b.librarianQuerier != nil && (strings.HasPrefix(intent.Name, "ai:") || strings.HasPrefix(intent.Name, "intent:summarize") || intent.Name == "ai:query") {
		// Try to extract a query from the payload to use for semantic search
		var query string
		var payloadData map[string]any
		if err := json.Unmarshal(intent.Payload, &payloadData); err == nil {
			if q, ok := payloadData["prompt"].(string); ok {
				query = q
			} else if q, ok := payloadData["text"].(string); ok {
				query = q
			} else if q, ok := payloadData["content"].(string); ok {
				query = q
			}
		}

		if query != "" {
			b.logger.Debug("performing semantic search for intent context", "intent", intent.Name, "query", query)
			contextContent, err := b.librarianQuerier(ctx, query)
			if err == nil && contextContent != "" {
				if msg.Metadata == nil {
					msg.Metadata = make(map[string]any)
				}
				msg.Metadata["semantic_context"] = contextContent
				b.logger.Debug("injected semantic context into intent", "intent", intent.Name, "size", len(contextContent))
			} else if err != nil {
				b.logger.Warn("librarian query failed in intent broker", "error", err)
			}
		}
	}

	b.router(ctx, msg)
	return nil
}
