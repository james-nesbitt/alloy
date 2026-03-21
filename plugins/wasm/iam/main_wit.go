//go:build wasip1 || wasm

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"./wit"
)

// Policy represents an access control policy.
type Policy struct {
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"` // e.g., ["plugin-chat:*", "plugin-projects:create"]
}

// AuthorizationRequest represents a request to check authorization.
type AuthorizationRequest struct {
	Actor  string `json:"actor"`
	Target string `json:"target"`
	Method string `json:"method"`
}

// PolicySetRequest represents a request to set a policy.
type PolicySetRequest struct {
	Policy Policy `json:"policy"`
}

// IdentitySetRequest represents a request to set an identity.
type IdentitySetRequest struct {
	Actor string `json:"actor"`
	Role  string `json:"role"`
}

// AuthorizationResponse represents an authorization response.
type AuthorizationResponse struct {
	Allowed bool `json:"allowed"`
}

var (
	plugin    *Plugin
	policies  = NewKVStore[Policy]("policies")
	identities = NewKVStore[string]("identities") // mapping of Actor -> Role
)

// KVStore provides type-safe KV storage.
type KVStore[T any] struct {
	prefix string
}

// NewKVStore creates a new KVStore instance.
func NewKVStore[T any](prefix string) *KVStore[T] {
	return &KVStore[T]{prefix: prefix}
}

// Get retrieves a value from KV storage.
func (s *KVStore[T]) Get(key string) (T, error) {
	var result T
	data, ok := plugin.KVGet(s.prefix + ":" + key)
	if !ok || data == nil {
		return result, fmt.Errorf("not found")
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	return result, nil
}

// Set stores a value in KV storage.
func (s *KVStore[T]) Set(key string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !plugin.KVSet(s.prefix+":"+key, data) {
		return fmt.Errorf("failed to set value")
	}
	return nil
}

func main() {
	// Create a new WIT-based plugin
	plugin = NewPlugin("iam").
		WithMetadata(
			"Identity and Access Management", 
			"Manages authentication and authorization for the system",
			"0.1.0", 
			"Alloy Team",
		).
		WithTags("iam", "security", "authentication", "authorization").
		WithCapability("check", "Check if an actor is authorized for an action").
		WithCapability("policy:set", "Set policy for a role").
		WithCapability("identity:set", "Set role for an actor")

	// Set up message handlers
	plugin.Handle("check", handleCheck)
	plugin.Handle("policy:set", handlePolicySet)
	plugin.Handle("identity:set", handleIdentitySet)

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "IAM plugin initializing")
		// Initialize default admin policy if not exists
		_, err := policies.Get("admin")
		if err != nil {
			defaultPolicy := Policy{
				Role:        "admin",
				Permissions: []string{"*"},
			}
			_ = policies.Set("admin", defaultPolicy)
			plugin.Log("info", "Initialized default admin policy")
		}
		return nil
	})

	// Run the plugin
	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

// handleCheck handles authorization checks.
func handleCheck(msg AlloyMessage) AlloyMessage {
	var req AuthorizationRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		plugin.Log("warn", "Invalid authorization request: "+err.Error())
		return Reply(msg, AuthorizationResponse{Allowed: false})
	}

	// Systems and internal kernel calls are self-authorized
	if req.Actor == "kernel" || req.Actor == "system" {
		return Reply(msg, AuthorizationResponse{Allowed: true})
	}

	// Get the actor's role
	role, err := identities.Get(req.Actor)
	if err != nil {
		// If no identity is found, use guest role
		role = "guest"
	}

	// Get the policy for the role
	policy, err := policies.Get(role)
	if err != nil {
		plugin.Log("warn", "No policy found for role: "+role)
		return Reply(msg, AuthorizationResponse{Allowed: false})
	}

	// Check if the action is allowed
	action := req.Target + ":" + req.Method
	allowed := false

	for _, perm := range policy.Permissions {
		if perm == "*" || perm == action || (strings.HasSuffix(perm, ":*") && strings.HasPrefix(action, perm[:len(perm)-1])) {
			allowed = true
			break
		}
	}

	plugin.Log("debug", fmt.Sprintf("Authorization check: actor=%s, role=%s, action=%s, allowed=%v", 
		req.Actor, role, action, allowed))

	return Reply(msg, AuthorizationResponse{Allowed: allowed})
}

// handlePolicySet handles setting policies.
func handlePolicySet(msg AlloyMessage) AlloyMessage {
	var req PolicySetRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ErrorReply(msg, "invalid_policy: "+err.Error())
	}

	// Save the policy
	if err := policies.Set(req.Policy.Role, req.Policy); err != nil {
		return ErrorReply(msg, "failed_to_save_policy: "+err.Error())
	}

	plugin.Log("info", fmt.Sprintf("Set policy for role: %s (%d permissions)", 
		req.Policy.Role, len(req.Policy.Permissions)))

	return Reply(msg, map[string]string{"status": "ok"})
}

// handleIdentitySet handles setting identities.
func handleIdentitySet(msg AlloyMessage) AlloyMessage {
	var req IdentitySetRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ErrorReply(msg, "invalid_identity: "+err.Error())
	}

	// Save the identity
	if err := identities.Set(req.Actor, req.Role); err != nil {
		return ErrorReply(msg, "failed_to_save_identity: "+err.Error())
	}

	plugin.Log("info", fmt.Sprintf("Set role %s for actor: %s", req.Role, req.Actor))

	return Reply(msg, map[string]string{"status": "ok"})
}