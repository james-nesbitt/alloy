package native

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

// CommandManager maintains a registry of all system-wide actions.
type CommandManager struct {
	mu       sync.RWMutex
	registry map[string]api.Registration
	logger   *slog.Logger
	route    func(context.Context, api.Message)
	provider func() []api.Registration
}

func NewCommandManager(logger *slog.Logger, provider func() []api.Registration) *CommandManager {
	return &CommandManager{
		registry: make(map[string]api.Registration),
		logger:   logger,
		provider: provider,
	}
}

func (c *CommandManager) SetRouter(r func(context.Context, api.Message)) {
	c.route = r
	// Subscribe to registration events
	c.route(context.Background(), api.Message{
		ID:        "sub-cm-reg",
		Type:      api.TypeRequest,
		Sender:    c.ID(),
		Target:    "events",
		Method:    "subscribe",
		Payload:   []byte(`{"topic":"component:registered"}`),
		Timestamp: time.Now().Unix(),
	})
}

func (c *CommandManager) ID() string { return "command-manager" }

func (c *CommandManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "register", Description: "Register component capabilities"},
		{Method: "list", Description: "List all registered commands"},
		{Method: "discovery:list", Description: "Alias for list"},
		{Method: "service:discovery", Description: "Unified service discovery"},
		{Method: "command-manager:discover", Description: "Alias for service discovery"},
		{Method: "discover", Description: "Unified service discovery (legacy)"},
		{Method: "component:registered", Description: "Event handler for registrations"},
	}
}

func (c *CommandManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "component:registered":
		var reg api.Registration
		if err := json.Unmarshal(msg.Payload, &reg); err != nil {
			c.logger.Error("failed to unmarshal component registration", "error", err, "payload", string(msg.Payload))
			return api.Message{}, err
		}
		c.logger.Info("component registered via event", "id", reg.ID, "type", reg.Type)
		c.mu.Lock()
		c.registry[reg.ID] = reg
		c.mu.Unlock()
		return api.Message{}, nil

	case "register":
		var reg api.Registration
		if err := json.Unmarshal(msg.Payload, &reg); err != nil {
			return api.Message{}, err
		}

		id := reg.ID
		if id == "" {
			id = msg.Sender
		}

		c.logger.Info("registering component in command-manager", "id", id, "type", reg.Type, "status", reg.Status)
		c.mu.Lock()

		existing := c.registry[id]
		if reg.Capabilities != nil {
			existing.Capabilities = reg.Capabilities
		}
		if reg.Status != "" {
			existing.Status = reg.Status
		}
		existing.ID = id
		existing.Type = reg.Type

		c.registry[id] = existing
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

	case "register-capability":
		var cap api.Capability
		if err := json.Unmarshal(msg.Payload, &cap); err != nil {
			return api.Message{}, err
		}

		c.mu.Lock()
		defer c.mu.Unlock()

		reg := c.registry[msg.Sender]
		reg.ID = msg.Sender
		reg.Type = "plugin"
		
		// Add or update capability
		found := false
		for i, c := range reg.Capabilities {
			if c.Method == cap.Method {
				reg.Capabilities[i] = cap
				found = true
				break
			}
		}
		if !found {
			reg.Capabilities = append(reg.Capabilities, cap)
		}

		c.registry[msg.Sender] = reg
		return api.Message{}, nil

	case "list", "discover", "service:discovery", "command-manager:discover", "discovery:list":
		var targets []api.Registration
		if c.provider != nil {
			targets = c.provider()
		} else {
			c.mu.RLock()
			for _, reg := range c.registry {
				targets = append(targets, reg)
			}
			c.mu.RUnlock()
		}

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
