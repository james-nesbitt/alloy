//go:build wasip1 || wasm

package main

import (
	. "github.com/jnesbitt/alloy-go/pkg/wasm/bindings/guest"
	. "github.com/jnesbitt/alloy-go/pkg/wasm/guest"
	"encoding/json"
)

// SecretStoreRequest represents a request to store a secret.
type SecretStoreRequest struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

// SecretGetRequest represents a request to get a secret.
type SecretGetRequest struct {
	ID string `json:"id"`
}

// SecretStoreResponse represents a response to storing a secret.
type SecretStoreResponse struct {
	Status string `json:"status"`
}

// SecretGetResponse represents a response to getting a secret.
type SecretGetResponse struct {
	Value string `json:"value"`
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
		WithCapability("store_secret", "Store a new secret").
		WithCapability("get_secret", "Retrieve a secret by ID")

	// Set up message handlers
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

// handleStoreSecret handles storing a secret.
func handleStoreSecret(msg AlloyMessage) AlloyMessage {
	var req SecretStoreRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request: "+err.Error())
	}

	// Store the secret
	secretKey := "secret:" + req.ID
	if !plugin.KVSet(secretKey, []byte(req.Value)) {
		return plugin.ErrorReply(msg, "failed_to_store_secret")
	}

	plugin.Log("info", "Stored secret: "+req.ID)

	return plugin.Reply(msg, SecretStoreResponse{Status: "stored"})
}

// handleGetSecret handles retrieving a secret.
func handleGetSecret(msg AlloyMessage) AlloyMessage {
	var req SecretGetRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request: "+err.Error())
	}

	// Get the secret
	secretKey := "secret:" + req.ID
	secretValue, ok := plugin.KVGet(secretKey)
	if !ok || secretValue == nil {
		return plugin.ErrorReply(msg, "secret_not_found")
	}

	plugin.Log("debug", "Retrieved secret: "+req.ID)

	return plugin.Reply(msg, SecretGetResponse{Value: string(secretValue)})
}
