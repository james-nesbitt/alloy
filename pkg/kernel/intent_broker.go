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

	// Phase 12: Intent Delegation & Collaboration
	delegations     map[string]*api.Delegation
	delegationsLock sync.RWMutex
}

// NewIntentBroker creates a new IntentBroker. If querier is non-nil, it will be used for automated context injection.
func NewIntentBroker(logger *slog.Logger, router func(ctx context.Context, msg api.Message), querier func(ctx context.Context, query string) (string, error)) *IntentBroker {
	return &IntentBroker{
		logger:           logger,
		providers:        make(map[string][]string),
		router:           router,
		librarianQuerier: querier,
		delegations:      make(map[string]*api.Delegation),
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
	var target string

	// Phase 12: Intent Targeting logic
	if intent.Target != "" && intent.Target != "*" {
		target = intent.Target
	}

	// Phase 12: Intent Delegation Logic - track multi-step task assignments
	if intent.Name == "intent:delegate" {
		var del api.Delegation
		if err := json.Unmarshal(intent.Payload, &del); err == nil {
			if del.ID == "" {
				del.ID = intent.ID
			}
			if del.Owner == "" {
				del.Owner = intent.Sender
			}
			if del.Status == "" {
				del.Status = "in_progress"
			}

			b.delegationsLock.Lock()
			// Track in parent if applicable (Verification chain tracking)
			if del.ParentID != "" {
				if parent, ok := b.delegations[del.ParentID]; ok {
					parent.Chain = append(parent.Chain, del.ID)
					b.logger.Debug("added sub-task to delegation chain", "parent", del.ParentID, "child", del.ID)
				}
			}

			// If target was found via providers but assignee is empty, use it.
			if del.Assignee == "" && target != "" {
				del.Assignee = target
			}

			b.delegations[del.ID] = &del
			b.delegationsLock.Unlock()

			b.logger.Info("tracking intent delegation", "id", del.ID, "owner", del.Owner, "assignee", del.Assignee)

			// Override target to assignee for delegation delivery
			if del.Assignee != "" {
				target = del.Assignee
			}
		}
	}

	// Phase 12: Delegation Updates
	if intent.Name == "intent:delegate:update" || intent.Name == "intent:delegate:complete" {
		var update struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Result string `json:"result,omitempty"`
		}
		if err := json.Unmarshal(intent.Payload, &update); err == nil && update.ID != "" {
			b.delegationsLock.Lock()
			if del, ok := b.delegations[update.ID]; ok {
				if intent.Name == "intent:delegate:complete" {
					del.Status = "complete"
				} else {
					del.Status = update.Status
				}
				b.logger.Info("delegation updated", "id", update.ID, "status", del.Status)
			}
			b.delegationsLock.Unlock()
		}
		// Treat update/complete as events to broadcast
		target = "*"
	}

	// Phase 12: Delegation Status Retrieval
	if intent.Name == "intent:delegate:status" {
		var req struct {
			ID   string `json:"id"`
			Deep bool   `json:"deep"`
		}
		if err := json.Unmarshal(intent.Payload, &req); err == nil {
			b.delegationsLock.RLock()
			del, ok := b.delegations[req.ID]
			if ok && req.Deep {
				// Create a deep copy for the response to avoid mutating the map under RLock
				delCopy := *del
				b.populateDeepDelegation(&delCopy, 0)
				del = &delCopy
			}
			b.delegationsLock.RUnlock()

			if ok {
				delData, _ := json.Marshal(del)
				b.router(ctx, api.Message{
					ID:        intent.ID + "-resp",
					Type:      api.TypeResponse,
					Sender:    "intent-broker",
					Target:    intent.Sender,
					Payload:   delData,
					Timestamp: time.Now().Unix(),
				})
				return nil
			}
		}
	}

	// Routine intent delivery if no explicit target was set/found
	if target == "" {
		b.mu.RLock()
		plugins, ok := b.providers[intent.Name]
		b.mu.RUnlock()

		if !ok || len(plugins) == 0 {
			// Phase 12: Broaden proactive interventions
			if intent.Name == "intent:propose" || intent.Name == "intent:suggest" {
				b.logger.Debug("routing proactive intent to broadcast", "intent", intent.Name)
				target = "*"
			} else {
				b.logger.Warn("no provider for intent", "intent", intent.Name)
				return fmt.Errorf("no provider for intent: %s", intent.Name)
			}
		} else {
			target = plugins[0]
		}
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

	// Phase 11: Semantic Context Injection
	if b.librarianQuerier != nil && (strings.HasPrefix(intent.Name, "ai:") || strings.HasPrefix(intent.Name, "intent:summarize") || intent.Name == "ai:query") {
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
			}
		}
	}

	b.router(ctx, msg)
	return nil
}

func (b *IntentBroker) populateDeepDelegation(del *api.Delegation, depth int) {
	if depth > 10 || len(del.Chain) == 0 {
		return
	}
	del.SubTasks = make([]*api.Delegation, 0, len(del.Chain))
	for _, subID := range del.Chain {
		if sub, ok := b.delegations[subID]; ok {
			subCopy := *sub
			b.populateDeepDelegation(&subCopy, depth+1)
			del.SubTasks = append(del.SubTasks, &subCopy)
		}
	}
}
