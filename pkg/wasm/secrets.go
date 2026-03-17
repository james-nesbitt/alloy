package wasm

import (
	"context"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

// SecretManager handles sensitive data.
type SecretManager struct {
	// In a real implementation, this would use encryption and an external vault.
	// For now, it's a stub that uses the StateStore indirectly if needed.
	secrets map[string]string
}

func NewSecretManager() *SecretManager {
	return &SecretManager{
		secrets: make(map[string]string),
	}
}

func (s *SecretManager) ID() string { return "plugin-secrets" }

func (s *SecretManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "get_secret", Description: "Retrieve a secret by ID"},
		{Method: "store_secret", Description: "Store a secret securely"},
	}
}

func (s *SecretManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	// In the future, this should check with plugin-iam to authorize the caller.
	switch msg.Method {
	case "get_secret":
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    s.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"value":"mock-secret-value"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	case "store_secret":
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    s.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"stored"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	}
	return api.Message{}, nil
}

func (s *SecretManager) Shutdown(ctx context.Context) error {
	return nil
}
