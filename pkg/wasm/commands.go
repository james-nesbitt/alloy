package wasm

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

type registration struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Capabilities []api.Capability `json:"capabilities,omitempty"`
}

// CommandManager maintains a registry of all system-wide actions.
type CommandManager struct {
	mu       sync.RWMutex
	registry map[string]registration
}

func NewCommandManager() *CommandManager {
	return &CommandManager{
		registry: make(map[string]registration),
	}
}

func (c *CommandManager) ID() string { return "plugin-command-manager" }

func (c *CommandManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "register", Description: "Register component capabilities"},
		{Method: "list", Description: "List all registered commands"},
		{Method: "discover", Description: "Unified service discovery"},
		{Method: "component:registered", Description: "Event handler for registrations"},
	}
}

func (c *CommandManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "component:registered":
		var reg registration
		if err := json.Unmarshal(msg.Payload, &reg); err != nil {
			return api.Message{}, err
		}
		c.mu.Lock()
		c.registry[reg.ID] = reg
		c.mu.Unlock()
		return api.Message{}, nil

	case "register":
		var caps []api.Capability
		if err := json.Unmarshal(msg.Payload, &caps); err != nil {
			return api.Message{}, err
		}
		c.mu.Lock()
		c.registry[msg.Sender] = registration{
			ID:           msg.Sender,
			Type:         "plugin",
			Capabilities: caps,
		}
		c.mu.Unlock()
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    c.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"registered"}`),
			Timestamp: time.Now().Unix(),
		}, nil

	case "list", "discover":
		c.mu.RLock()
		var targets []registration
		for _, reg := range c.registry {
			targets = append(targets, reg)
		}
		c.mu.RUnlock()

		payload, _ := json.Marshal(map[string]any{
			"targets": targets,
		})
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    c.ID(),
			Target:    msg.Sender,
			Payload:   payload,
			Timestamp: time.Now().Unix(),
		}, nil
	}
	return api.Message{}, nil
}

func (c *CommandManager) Shutdown(ctx context.Context) error {
	return nil
}
