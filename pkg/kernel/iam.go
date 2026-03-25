package kernel

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

// Policy represents an access control policy for a role.
type Policy struct {
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

// IdentityManager implements authorization enforcement for the kernel.
type IdentityManager struct {
	logger     *slog.Logger
	state      storage.StateStore
	mu         sync.RWMutex
	policies   map[string]Policy
	identities map[string]string // actor -> role
}

func NewIdentityManager(ctx context.Context, logger *slog.Logger, state storage.StateStore) (*IdentityManager, error) {
	iam := &IdentityManager{
		logger:     logger,
		state:      state,
		policies:   make(map[string]Policy),
		identities: make(map[string]string),
	}

	// Bootstrap default roles
	iam.policies["admin"] = Policy{Role: "admin", Permissions: []string{"*"}}
	iam.policies["guest"] = Policy{Role: "guest", Permissions: []string{"*"}}
	// Bootstrapped guest: health, events, etc.
	// iam.policies["guest"] = Policy{Role: "guest", Permissions: []string{
	// 	"health:*", "events:*", "chat:*", "buffer:*", "command-manager:*", "iam:check", "kernel:*", "kv:*", "storage:*", "network:*", "cache:*", "doc:*", "ai:*",
	// }}

	// Bootstrap system identities
	iam.identities["system"] = "admin"
	iam.identities["kernel"] = "admin"
	iam.identities["iam"] = "admin"
	iam.identities["events"] = "admin"
	iam.identities["command-manager"] = "admin"
	iam.identities["wasm-manager"] = "admin"
	iam.identities["logger"] = "admin"

	return iam, nil
}

func (i *IdentityManager) ID() string { return "iam" }

func (i *IdentityManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "authorize", Description: "Check routing permissions"},
		{Method: "check", Description: "Check if an actor is authorized for an action"},
		{Method: "policy:set", Description: "Update or create a role policy"},
		{Method: "identity:set", Description: "Assign a role to an actor"},
	}
}

func (i *IdentityManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "check", "authorize":
		var req struct {
			Actor  string `json:"actor"`
			Target string `json:"target"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			// fallback to old simple check?
			return api.Message{}, err
		}

		allowed := i.Authorize(req.Actor, req.Target, req.Method)
		return i.reply(msg, map[string]any{"allowed": allowed}), nil

	case "policy:set":
		var req struct {
			Policy Policy `json:"policy"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}
		i.mu.Lock()
		i.policies[req.Policy.Role] = req.Policy
		i.mu.Unlock()
		i.logger.Info("IAM policy updated", "role", req.Policy.Role, "perms", len(req.Policy.Permissions))
		return i.reply(msg, map[string]string{"status": "ok"}), nil

	case "identity:set":
		var req struct {
			Actor string `json:"actor"`
			Role  string `json:"role"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}
		i.mu.Lock()
		i.identities[req.Actor] = req.Role
		i.mu.Unlock()
		i.logger.Info("IAM identity updated", "actor", req.Actor, "role", req.Role)
		return i.reply(msg, map[string]string{"status": "ok"}), nil

	case "allow": // Legacy/Simple
		var req struct {
			Sender string `json:"sender"`
			Target string `json:"target"`
		}
		json.Unmarshal(msg.Payload, &req)
		i.mu.Lock()
		role := "legacy-" + req.Sender
		p := i.policies[role]
		p.Role = role
		p.Permissions = append(p.Permissions, req.Target+":*")
		i.policies[role] = p
		i.identities[req.Sender] = role
		i.mu.Unlock()
		return i.reply(msg, map[string]string{"status": "policy-added"}), nil
	}
	return api.Message{}, nil
}

func (i *IdentityManager) reply(msg api.Message, payload any) api.Message {
	data, _ := json.Marshal(payload)
	return api.Message{
		ID:        msg.ID + "-resp",
		Type:      api.TypeResponse,
		Sender:    i.ID(),
		Target:    msg.Sender,
		Payload:   data,
		Timestamp: time.Now().Unix(),
	}
}

func (i *IdentityManager) Authorize(actor, target, method string) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// 1. Get role
	role, ok := i.identities[actor]
	if !ok {
		role = "guest"
	}

	// 2. Get policy
	policy, ok := i.policies[role]
	if !ok {
		return false
	}

	// 3. Check permissions
	action := target + ":" + method
	for _, perm := range policy.Permissions {
		if perm == "*" {
			return true
		}
		if perm == action {
			return true
		}
		if strings.HasSuffix(perm, ":*") && strings.HasPrefix(action, perm[:len(perm)-1]) {
			return true
		}
	}

	i.logger.Warn("IAM denied access", "actor", actor, "role", role, "target", target, "method", method)
	return false
}

func (i *IdentityManager) Shutdown(ctx context.Context) error { return nil }
