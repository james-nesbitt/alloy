package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

// IdentityManager implements authorization enforcement for the kernel.
type IdentityManager struct {
	logger *slog.Logger
	state  storage.StateStore
	mu     sync.RWMutex

	// policies maps sender to allowed targets
	policies map[string]map[string]bool
}

func NewIdentityManager(ctx context.Context, logger *slog.Logger, state storage.StateStore) (*IdentityManager, error) {
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
		{Method: "check", Description: "Check if an actor is authorized for an action"},
	}
}

func (i *IdentityManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "check":
		var req struct {
			Actor  string `json:"actor"`
			Target string `json:"target"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		allowed := i.Authorize(req.Actor, req.Target, req.Method)

		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  i.ID(),
			Target:  msg.Sender,
			Payload: []byte(fmt.Sprintf(`{"allowed":%v}`, allowed)),
			Timestamp: time.Now().Unix(),
		}, nil

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
			Timestamp: time.Now().Unix(),
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
			Timestamp: time.Now().Unix(),
		}, nil
	}
	return api.Message{}, nil
}

// Authorize checks if an actor has permission to perform an action.
func (i *IdentityManager) Authorize(actor, target, method string) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// 1. Check if actor has policies
	targetPolicies, hasPolicies := i.policies[actor]
	if !hasPolicies {
		// By default, for now, we allow unknown actors if not explicitly restricted.
		return true
	}

	// 2. Check if wildcard allowed
	if targetPolicies["*"] {
		return true
	}

	// 3. Check if explicit target allowed
	if targetPolicies[target] {
		return true
	}

	i.logger.Warn("IAM denied access", "actor", actor, "target", target, "method", method)
	return false
}

func (i *IdentityManager) Shutdown(ctx context.Context) error {
	return nil
}
