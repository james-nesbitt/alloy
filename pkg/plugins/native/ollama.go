package native

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

type OllamaProvider struct {
	logger *slog.Logger
	state  storage.StateStore
}

func NewOllamaProvider(ctx context.Context, logger *slog.Logger, state storage.StateStore) (api.Plugin, error) {
	return &OllamaProvider{
		logger: logger,
		state:  state,
	}, nil
}

func (o *OllamaProvider) ID() string { return "ollama" }

func (o *OllamaProvider) Capabilities() []api.Capability {
	return []api.Capability{
		{
			Method:      "generate",
			Description: "Generate text using local Ollama model",
			Annotations: map[string]string{
				"type":     "llm",
				"provider": "ollama",
			},
		},
		{
			Method:      "models",
			Description: "List available local models",
		},
	}
}

func (o *OllamaProvider) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "generate":
		var req struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		res := map[string]string{
			"response": "Local Ollama response for prompt: " + req.Prompt,
		}
		payload, _ := json.Marshal(res)
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    o.ID(),
			Target:    msg.Sender,
			Payload:   payload,
			Timestamp: time.Now().Unix(),
		}, nil

	case "models":
		res := []string{"llama3", "mistral", "phi3"}
		payload, _ := json.Marshal(map[string]any{"models": res})
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    o.ID(),
			Target:    msg.Sender,
			Payload:   payload,
			Timestamp: time.Now().Unix(),
		}, nil
	}
	return api.Message{}, nil
}

func (o *OllamaProvider) Shutdown(ctx context.Context) error { return nil }
