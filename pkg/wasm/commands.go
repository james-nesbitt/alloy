package wasm

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

// CommandManager maintains a registry of all system-wide actions.
type CommandManager struct {
	mu       sync.RWMutex
	registry map[string][]api.Capability
}

func NewCommandManager() *CommandManager {
	return &CommandManager{
		registry: make(map[string][]api.Capability),
	}
}

func (c *CommandManager) ID() string { return "plugin-command-manager" }

func (c *CommandManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "register", Description: "Register component capabilities"},
		{Method: "list", Description: "List all registered commands"},
	}
}

func (c *CommandManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "register":
		var caps []api.Capability
		if err := json.Unmarshal(msg.Payload, &caps); err != nil {
			return api.Message{}, err
		}
		c.mu.Lock()
		c.registry[msg.Sender] = caps
		c.mu.Unlock()
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    c.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"registered"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	case "list":
		c.mu.RLock()
		payload, _ := json.Marshal(c.registry)
		c.mu.RUnlock()
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
