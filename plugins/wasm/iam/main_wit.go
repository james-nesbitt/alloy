//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Policy represents an access control policy.
type Policy struct {
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"` // e.g., ["chat:*", "buffer:read:public-*"]
}

// AuthorizationRequest represents a request to check authorization.
type AuthorizationRequest struct {
	Actor    string `json:"actor"`
	Target   string `json:"target"`
	Method   string `json:"method"`
	Resource string `json:"resource,omitempty"` // e.g. buffer-id, project-id
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
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

var (
	plugin    *Plugin
	policies  = NewKVStore[Policy]("policies")
	identities = NewKVStore[string]("identities")
)

// KVStore provides type-safe KV storage.
type KVStore[T any] struct {
	prefix string
}

func NewKVStore[T any](prefix string) *KVStore[T] {
	return &KVStore[T]{prefix: prefix}
}

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
	plugin = NewPlugin("iam").
		WithMetadata(
			"Identity and Access Management", 
			"Enterprise-grade security and granular RBAC",
			"0.2.0", 
			"Alloy Team",
		).
		WithTags("iam", "security", "rbac", "auditing").
		WithCapability("check", "Check if an actor is authorized for an action").
		WithCapability("policy:set", "Set policy for a role").
		WithCapability("identity:set", "Set role for an actor").
		WithCapability("audit:log", "Get recent security events")

	plugin.Handle("check", handleCheck)
	plugin.Handle("policy:set", handlePolicySet)
	plugin.Handle("identity:set", handleIdentitySet)

	plugin.OnInit(func() error {
		plugin.Log("info", "IAM Security hardening active")
		
		defaultRoles := map[string][]string{
			"admin": {"*"},
			"guest": {
				"health:*",
				"command-manager:discover",
				"events:*",
				"chat:*",
				"project:*",
				"ai:*",
				"iam:check",
				"secrets:*",
				"buffer:read:*",
				"buffer:write:public-*", // Guests can only write to public buffers
				"presence:*",
			},
		}

		for role, perms := range defaultRoles {
			_, err := policies.Get(role)
			if err != nil {
				_ = policies.Set(role, Policy{Role: role, Permissions: perms})
				plugin.Log("info", "Initialized default "+role+" policy")
			}
		}

		return nil
	})

	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

func handleCheck(msg AlloyMessage) AlloyMessage {
	var req AuthorizationRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.Reply(msg, AuthorizationResponse{Allowed: false, Reason: "malformed_request"})
	}

	if req.Actor == "kernel" || req.Actor == "system" {
		return plugin.Reply(msg, AuthorizationResponse{Allowed: true})
	}

	role, err := identities.Get(req.Actor)
	if err != nil {
		role = "guest"
	}

	policy, err := policies.Get(role)
	if err != nil {
		return plugin.Reply(msg, AuthorizationResponse{Allowed: false, Reason: "no_policy_for_role"})
	}

	action := req.Target + ":" + req.Method
	if req.Resource != "" {
		action = action + ":" + req.Resource
	}
	
	allowed := false
	for _, perm := range policy.Permissions {
		if checkPermission(perm, action) {
			allowed = true
			break
		}
	}

	// Auditing
	logLevel := "debug"
	if !allowed {
		logLevel = "warn"
	}
	
	auditMsg := fmt.Sprintf("IAM: actor=%s (role=%s) action=%s resource=%s allowed=%v", 
		req.Actor, role, req.Target+":"+req.Method, req.Resource, allowed)
	plugin.Log(logLevel, auditMsg)

	// Emit audit event for security monitoring
	auditPayload, _ := json.Marshal(map[string]interface{}{
		"topic": "system:audit",
		"event": "authorization_check",
		"data": map[string]interface{}{
			"actor":    req.Actor,
			"role":     role,
			"target":   req.Target,
			"method":   req.Method,
			"resource": req.Resource,
			"allowed":  allowed,
			"time":     time.Now().Unix(),
		},
	})
	plugin.RouteMessage(AlloyMessage{
		Id:      fmt.Sprintf("audit-%d", time.Now().UnixNano()),
		Method:  "publish",
		Sender:  "iam",
		Target:  Some("events"),
		Payload: auditPayload,
	})

	return plugin.Reply(msg, AuthorizationResponse{Allowed: allowed})
}

func checkPermission(perm, action string) bool {
	if perm == "*" {
		return true
	}
	
	// Handle wildcards like "buffer:read:*" or "chat:*"
	permParts := strings.Split(perm, ":")
	actionParts := strings.Split(action, ":")
	
	for i, pPart := range permParts {
		if pPart == "*" {
			return true // Wildcard match for everything after
		}
		if i >= len(actionParts) {
			return false
		}
		if pPart != actionParts[i] {
			return false
		}
	}
	
	// If we matched all parts of the permission precisely, it must be the same length as action
	return len(permParts) == len(actionParts)
}

func handlePolicySet(msg AlloyMessage) AlloyMessage {
	var req PolicySetRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_policy: "+err.Error())
	}

	if err := policies.Set(req.Policy.Role, req.Policy); err != nil {
		return plugin.ErrorReply(msg, "failed_to_save_policy: "+err.Error())
	}

	plugin.Log("info", fmt.Sprintf("Policy updated: %s", req.Policy.Role))
	return plugin.Reply(msg, map[string]string{"status": "ok"})
}

func handleIdentitySet(msg AlloyMessage) AlloyMessage {
	var req IdentitySetRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_identity: "+err.Error())
	}

	if err := identities.Set(req.Actor, req.Role); err != nil {
		return plugin.ErrorReply(msg, "failed_to_save_identity: "+err.Error())
	}

	plugin.Log("info", fmt.Sprintf("Identity updated: %s -> %s", req.Actor, req.Role))
	return plugin.Reply(msg, map[string]string{"status": "ok"})
}
