package native

import (
	"context"
	"encoding/json"
	"log/slog"
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
	logger   *slog.Logger
	route    func(context.Context, api.Message)
}

func NewCommandManager(logger *slog.Logger) *CommandManager {
	return &CommandManager{
		registry: make(map[string]registration),
		logger:   logger,
	}
}

func (c *CommandManager) SetRouter(r func(context.Context, api.Message)) {
	c.route = r
	// Subscribe to registration events
	go func() {
		// Small delay to ensure event manager is likely up
		time.Sleep(100 * time.Millisecond)
		c.route(context.Background(), api.Message{
			ID:        "sub-cm-reg",
			Type:      api.TypeRequest,
			Sender:    c.ID(),
			Target:    "plugin-events",
			Method:    "subscribe",
			Payload:   []byte(`{"topic":"component:registered"}`),
			Timestamp: time.Now().Unix(),
		})
	}()
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
		var reg registration
		if err := json.Unmarshal(msg.Payload, &reg); err != nil {
			return api.Message{}, err
		}
		
		id := reg.ID
		if id == "" {
			id = msg.Sender
		}

		c.logger.Info("registering component in command-manager", "id", id, "type", reg.Type)
		c.mu.Lock()
		c.registry[id] = registration{
			ID:           id,
			Type:         reg.Type,
			Capabilities: reg.Capabilities,
		}
		c.mu.Unlock()
		if msg.Sender == "" {
			return api.Message{}, nil
		}
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
