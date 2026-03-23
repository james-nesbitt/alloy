//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/build/gen/bindings/guest"
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"strings"
)

// SecretStoreRequest represents a request to store a secret.
type SecretStoreRequest struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	Scope string `json:"scope,omitempty"` // "user", "project", "team"
}

// SecretGetRequest represents a request to get a secret.
type SecretGetRequest struct {
	ID    string `json:"id"`
	Scope string `json:"scope,omitempty"` 
}

// SecretStoreResponse represents a response to storing a secret.
type SecretStoreResponse struct {
	Status string `json:"status"`
	Scope  string `json:"scope"`
}

// SecretGetResponse represents a response to getting a secret.
type SecretGetResponse struct {
	Value string `json:"value"`
	Scope string `json:"scope"`
}

// SecretListResponse represents a response to listing secrets.
type SecretListResponse struct {
	Secrets []string `json:"secrets"`
}

var plugin *Plugin

func main() {
	// Create a new WIT-based plugin
	plugin = NewPlugin("secrets").
		WithMetadata(
			"Secrets Manager", 
			"Manages sensitive data and secrets for the system",
			"0.1.0", 
			"Alloy Team",
		).
		WithTags("secrets", "security", "credentials").
		WithCapability("store", "Store a new secret").WithShortcut("s s").
		WithCapability("get", "Retrieve a secret by ID").WithShortcut("s g").
		WithCapability("list", "List secrets in current scope").WithShortcut("s l")

	// Set up message handlers
	plugin.Handle("store", handleStoreSecret)
	plugin.Handle("get", handleGetSecret)
	plugin.Handle("list", handleListSecrets)
	
	// Legacy support
	plugin.Handle("store_secret", handleStoreSecret)
	plugin.Handle("get_secret", handleGetSecret)

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "Secrets plugin initializing")
		return nil
	})

	// Run the plugin
	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

func getScopePrefix(requestScope string) string {
	scope := requestScope
	if scope == "" {
		scope = "project" // Default to project scope
	}

	switch scope {
	case "user":
		return "user:secret:"
	case "team":
		ws, ok := plugin.GetActiveWorkspace()
		if ok && ws.TeamId.IsSome() {
			return "team:" + ws.TeamId.Unwrap() + ":secret:"
		}
		return "global:team:secret:"
	case "project":
		ws, ok := plugin.GetActiveWorkspace()
		if ok {
			return "project:" + ws.Id + ":secret:"
		}
		return "global:project:secret:"
	default:
		return "global:secret:"
	}
}

// handleStoreSecret handles storing a secret.
func handleStoreSecret(msg AlloyMessage) AlloyMessage {
	var req SecretStoreRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request: "+err.Error())
	}

	prefix := getScopePrefix(req.Scope)
	secretKey := prefix + req.ID
	if !plugin.KVSet(secretKey, []byte(req.Value)) {
		return plugin.ErrorReply(msg, "failed_to_store_secret")
	}

	plugin.Log("info", "Stored secret: "+req.ID+" in scope "+prefix)

	return plugin.Reply(msg, SecretStoreResponse{Status: "stored", Scope: prefix})
}

// handleGetSecret handles retrieving a secret.
func handleGetSecret(msg AlloyMessage) AlloyMessage {
	var req SecretGetRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request: "+err.Error())
	}

	// Try specific scope if provided, else try hierarchical lookup
	if req.Scope != "" {
		prefix := getScopePrefix(req.Scope)
		val, ok := plugin.KVGet(prefix + req.ID)
		if ok && val != nil {
			return plugin.Reply(msg, SecretGetResponse{Value: string(val), Scope: req.Scope})
		}
	} else {
		// Hierarchical lookup: Project -> Team -> User -> Global
		scopes := []string{"project", "team", "user", "global"}
		for _, s := range scopes {
			prefix := getScopePrefix(s)
			val, ok := plugin.KVGet(prefix + req.ID)
			if ok && val != nil {
				return plugin.Reply(msg, SecretGetResponse{Value: string(val), Scope: s})
			}
		}
	}

	return plugin.ErrorReply(msg, "secret_not_found")
}

// handleListSecrets handles listing secrets.
func handleListSecrets(msg AlloyMessage) AlloyMessage {
	var req struct {
		Scope string `json:"scope"`
	}
	_ = json.Unmarshal(msg.Payload, &req)

	prefix := getScopePrefix(req.Scope)
	keys := plugin.KVList(prefix)
	
	names := make([]string, 0, len(keys))
	for _, k := range keys {
		names = append(names, strings.TrimPrefix(k, prefix))
	}

	return plugin.Reply(msg, SecretListResponse{Secrets: names})
}
