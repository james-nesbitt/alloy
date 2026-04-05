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

// ActorPrefix is the required prefix for agent identities.
const ActorPrefix = "actor:"

// IdentityManager implements authorization enforcement for the kernel.
type IdentityManager struct {
	logger   *slog.Logger
	state    storage.StateStore
	mu       sync.RWMutex
	insecure bool
	policies map[string]Policy

	identities      map[string]string              // actor -> role
	namespaceGrants map[string]map[string][]string // actor -> namespace -> capabilities
}

func NewIdentityManager(ctx context.Context, logger *slog.Logger, state storage.StateStore) (*IdentityManager, error) {
	iam := &IdentityManager{
		logger:          logger,
		state:           state,
		policies:        make(map[string]Policy),
		identities:      make(map[string]string),
		namespaceGrants: make(map[string]map[string][]string),
	}

	// Bootstrap default roles
	iam.policies["admin"] = Policy{Role: "admin", Permissions: []string{"*"}}

	// Phase 12: Core Alloy Actor Roles
	iam.policies["developer"] = Policy{Role: "developer", Permissions: []string{
		"*/buffer:*",           // Managed buffers in any namespace
		"buffer:list",          // Discovery
		"command-manager:exec", // Running tools
		"events:*",             // Monitoring stream
		"index:knowledge:*",    // RAG access
		"project:*",            // Project metadata
		"wasm-manager:*",       // Plugin control
		"kernel:*",             // Basic system calls
		"iam:check",            // Perm self-check
	}}

	iam.policies["auditor"] = Policy{Role: "auditor", Permissions: []string{
		"*/buffer:read",          // Read-only in any namespace
		"buffer:list",            // Discovery
		"index:knowledge:search", // RAG search
		"iam:audit",              // Specific audit capabilities
		"events:listen",          // Monitoring
		"logger:read",            // Log access
		"health:*",               // System health
		"iam:check",
	}}

	iam.policies["actor"] = Policy{Role: "actor", Permissions: []string{
		"health:check",
		"chat:*",
		"buffer:read",
		"index:knowledge:search",
		"intent:propose",
		"iam:check", // Standard self-check capability
	}}

	iam.policies["reviewer"] = Policy{Role: "reviewer", Permissions: []string{
		"*/buffer:read",       // Read-only in any namespace
		"buffer:list",         // Discovery
		"project:review",      // Approval flows
		"project:merge",       // Finalizing changes
		"iam:policy:check",    // Verifying policies
		"wasm-manager:status", // Verification of plugin state
		"iam:check",
	}}

	// Explicit guest permissions for core functionality
	iam.policies["guest"] = Policy{Role: "guest", Permissions: []string{
		"health:*",               // Any user can check health status
		"command-manager:*",      // Discovery is public
		"events:*",               // Pub/Sub discovery and usage
		"iam:check",              // Checking own permissions
		"chat:*",                 // Basic interaction
		"buffer:read",            // Public data
		"buffer:list",            // Public data
		"buffer:buffer:read",     // Capability-based
		"buffer:buffer:list",     // Capability-based
		"kernel:*",               // Basic system calls (e.g. registry read)
		"logger:*",               // Sending logs
		"wasm-manager:status",    // Seeing plugin status
		"widget-manager:*",       // Dashboard updates
		"omni-palette:*",         // Search usage
		"index:knowledge:search", // Search usage
	}}

	// Bootstrap system identities
	iam.identities["system"] = "admin"
	iam.identities["kernel"] = "admin"
	iam.identities["iam"] = "admin"
	iam.identities["events"] = "admin"
	iam.identities["command-manager"] = "admin"
	iam.identities["wasm-manager"] = "admin"
	iam.identities["widget-manager"] = "admin"
	iam.identities["logger"] = "admin"
	iam.identities["admin-user"] = "admin"
	iam.identities["admin-sim"] = "admin"
	iam.identities["user"] = "admin"
	iam.identities["test-waiter"] = "admin"
	iam.identities["test-client"] = "admin"
	iam.identities["test-frontend"] = "admin"
	iam.identities["tui-sim"] = "admin"
	iam.identities["gui-sim"] = "admin"
	iam.identities["web-host-sim"] = "admin"
	iam.identities["test-user"] = "admin"
	iam.identities["dashboard-mock"] = "admin"
	iam.identities["event-catcher"] = "admin"

	// Plugin identities (when acting as actors)
	iam.identities["ai"] = "admin"
	iam.identities["chat"] = "admin"
	iam.identities["buffer"] = "admin"
	iam.identities["health"] = "admin"
	iam.identities["test-health"] = "admin"
	iam.identities["secrets"] = "admin"
	iam.identities["tasks"] = "admin"
	iam.identities["index"] = "admin"
	iam.identities["project"] = "admin"
	iam.identities["omni-palette"] = "admin"

	// Phase 12: Bootstrapped Actor identities
	iam.identities["actor:claudine"] = "developer"
	iam.identities["actor:auditor"] = "auditor"
	iam.identities["actor:reviewer"] = "reviewer"

	return iam, nil
}

func (i *IdentityManager) SetInsecure(insecure bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.insecure = insecure
}

func (i *IdentityManager) ID() string { return "iam" }

// IsActor checks if the given identity string represents an AI actor.
func (i *IdentityManager) IsActor(identity string) bool {
	return strings.HasPrefix(identity, ActorPrefix)
}

// PolicyTemplates returns standard policies for actor roles.
func (i *IdentityManager) PolicyTemplates() map[string]Policy {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return map[string]Policy{
		"developer": i.policies["developer"],
		"auditor":   i.policies["auditor"],
		"reviewer":  i.policies["reviewer"],
	}
}

func (i *IdentityManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "authorize", Description: "Check routing permissions"},
		{Method: "check", Description: "Check if an actor is authorized for an action"},
		{Method: "policy:set", Description: "Update or create a role policy"},
		{Method: "policy:templates", Description: "Get standard policy templates"},
		{Method: "identity:set", Description: "Assign a role to an actor"},
		{Method: "grant_namespace_role", Description: "Grant ephemeral role capabilities in a namespace"},
	}
}

func (i *IdentityManager) AssignRole(actor, role string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.identities[actor] = role
}

func (i *IdentityManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "check", "authorize":
		var req struct {
			Actor   string `json:"actor"`
			Target  string `json:"target"`
			Method  string `json:"method"`
			Context string `json:"context,omitempty"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		allowed := i.AuthorizeWithContext(req.Actor, req.Target, req.Method, req.Context)
		return i.reply(msg, map[string]any{"allowed": allowed}), nil

	case "grant_namespace_role":
		// This must be from a trusted system service (project, kernel)
		// We'll enforce this via standard authorize, but internal checks could be stricter
		var req struct {
			Actor        string   `json:"actor"`
			Namespace    string   `json:"namespace"`
			Capabilities []string `json:"capabilities"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		i.mu.Lock()
		if _, ok := i.namespaceGrants[req.Actor]; !ok {
			i.namespaceGrants[req.Actor] = make(map[string][]string)
		}
		i.namespaceGrants[req.Actor][req.Namespace] = req.Capabilities
		i.mu.Unlock()

		i.logger.Info("IAM namespace grant active", "actor", req.Actor, "ns", req.Namespace, "caps", len(req.Capabilities))
		return i.reply(msg, map[string]string{"status": "ok"}), nil

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

	case "policy:templates":
		return i.reply(msg, i.PolicyTemplates()), nil

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
	return i.AuthorizeWithContext(actor, target, method, "")
}

func (i *IdentityManager) AuthorizeWithContext(actor, target, method, contextID string) bool {
	i.mu.RLock()
	insecure := i.insecure
	i.mu.RUnlock()
	if insecure {
		return true
	}

	i.mu.RLock()

	defer i.mu.RUnlock()

	// Formalize Actor status
	isAgent := i.IsActor(actor)

	// 1. Get role (Global)
	role, ok := i.identities[actor]
	if !ok {
		if i.IsActor(actor) {
			role = "actor"
		} else {
			role = "guest"
		}
	}

	action := target + ":" + method

	// 2. Check Namespace Permissions first (Ephemeral grants)
	if contextID != "" {
		if ns, ok := i.namespaceGrants[actor]; ok {
			if caps, ok := ns[contextID]; ok {
				for _, perm := range caps {
					if i.matchAction(perm, action) {
						return true
					}
				}
			}
		}
	}

	// 3. Get policy (Global)
	policy, ok := i.policies[role]
	if !ok {
		return false
	}

	// 4. Check global permissions (including namespaced patterns)
	for _, perm := range policy.Permissions {
		if i.matchPerm(perm, action, contextID) {
			return true
		}
	}

	if isAgent {
		i.logger.Warn("IAM actor denied access", "actor", actor, "role", role, "target", target, "method", method, "ctx", contextID)
	} else {
		i.logger.Warn("IAM denied access", "actor", actor, "role", role, "target", target, "method", method, "ctx", contextID)
	}
	return false
}

func (i *IdentityManager) matchPerm(perm, action, contextID string) bool {
	// 1. Check for namespace prefix in perm (e.g., "prj-123/buffer:read")
	if strings.Contains(perm, "/") {
		parts := strings.SplitN(perm, "/", 2)
		nsPattern := parts[0]
		rest := parts[1]

		// Namespace wildcard support (e.g. "*/buffer:read")
		nsMatch := false
		if nsPattern == "*" {
			nsMatch = true
		} else if strings.HasSuffix(nsPattern, "*") {
			nsMatch = strings.HasPrefix(contextID, nsPattern[:len(nsPattern)-1])
		} else {
			nsMatch = (nsPattern == contextID)
		}

		if !nsMatch {
			return false
		}

		// Match the rest against the action
		return i.matchAction(rest, action)
	}

	// 2. Global perm applies regardless of context (e.g. "buffer:read" matches all)
	return i.matchAction(perm, action)
}

func (i *IdentityManager) matchAction(pattern, action string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == action {
		return true
	}
	// Support both "target:*" and "target*"
	if strings.HasSuffix(pattern, ":*") {
		return strings.HasPrefix(action, pattern[:len(pattern)-1])
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(action, pattern[:len(pattern)-1])
	}
	return false
}

func (i *IdentityManager) Shutdown(ctx context.Context) error { return nil }
