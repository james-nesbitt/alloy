package native

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

// IdentityManager implements a native plugin that controls message routing permissions.
type IdentityManager struct {
	logger *slog.Logger
	state  storage.StateStore
	mu     sync.RWMutex

	// policies maps sender to allowed targets
	policies map[string]map[string]bool
}

func NewIdentityManager(ctx context.Context, logger *slog.Logger, state storage.StateStore) (api.Plugin, error) {
	iam := &IdentityManager{
		logger:   logger,
		state:    state,
		policies: make(map[string]map[string]bool),
	}

	// Initial bootstrap policies
	iam.policies["system"] = map[string]bool{"*": true}
	iam.policies["kernel"] = map[string]bool{"*": true}
	iam.policies["registry"] = map[string]bool{"*": true}
	iam.policies["command-manager"] = map[string]bool{"*": true}

	return iam, nil
}

func (i *IdentityManager) ID() string { return "iam" }

func (i *IdentityManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "authorize", Description: "Check routing permissions"},
	}
}

func (i *IdentityManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "allow":
		var req struct {
			Sender string `json:"sender"`
			Target string `json:"target"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}
		i.mu.Lock()
		if _, ok := i.policies[req.Sender]; !ok {
			i.policies[req.Sender] = make(map[string]bool)
		}
		i.policies[req.Sender][req.Target] = true
		i.mu.Unlock()
		i.logger.Info("policy updated: ALLOW", "sender", req.Sender, "target", req.Target)
		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  i.ID(),
			Target:  msg.Sender,
			Payload: []byte(`{"status":"policy-added"}`),
		}, nil

	case "deny":
		var req struct {
			Sender string `json:"sender"`
			Target string `json:"target"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}
		i.mu.Lock()
		if _, ok := i.policies[req.Sender]; ok {
			delete(i.policies[req.Sender], req.Target)
			if len(i.policies[req.Sender]) == 0 {
				delete(i.policies, req.Sender)
			}
		}
		i.mu.Unlock()
		i.logger.Info("policy updated: DENY", "sender", req.Sender, "target", req.Target)
		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  i.ID(),
			Target:  msg.Sender,
			Payload: []byte(`{"status":"policy-removed"}`),
		}, nil
	}
	return api.Message{}, nil
}

func (i *IdentityManager) PreRoute(ctx context.Context, msg api.Message) (api.Message, bool, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// 1. Check if sender has policies
	targetPolicies, hasPolicies := i.policies[msg.Sender]
	if !hasPolicies {
		// By default, for now, we allow unknown senders if not explicitly restricted.
		// In a production scenario, we'd probably have a "default allow/deny" policy.
		return msg, true, nil
	}

	// 2. Check if wildcard allowed
	if targetPolicies["*"] {
		return msg, true, nil
	}

	// 3. Check if explicit target allowed
	if targetPolicies[msg.Target] {
		return msg, true, nil
	}

	i.logger.Warn("IAM denied routing", "sender", msg.Sender, "target", msg.Target, "method", msg.Method)
	return msg, false, nil
}

func (i *IdentityManager) Shutdown(ctx context.Context) error { return nil }
