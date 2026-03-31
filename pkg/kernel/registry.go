package kernel

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

// WidgetManager maintains a registry of dashboard widgets.
type WidgetManager struct {
	mu      sync.RWMutex
	widgets map[string]api.Widget
	logger  *slog.Logger
	router  func(context.Context, api.Message)
	store   storage.StateStore
}

func NewWidgetManager(logger *slog.Logger, store storage.StateStore) *WidgetManager {
	wm := &WidgetManager{
		widgets: make(map[string]api.Widget),
		logger:  logger,
		store:   store,
	}
	wm.load()
	return wm
}

func (w *WidgetManager) SetRouter(r func(context.Context, api.Message)) {
	w.router = r
}

func (w *WidgetManager) ID() string { return "widget-manager" }

func (w *WidgetManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "register", Description: "Register a dashboard widget"},
		{Method: "unregister", Description: "Unregister a dashboard widget"},
		{Method: "update", Description: "Update widget content"},
		{Method: "list", Description: "List all widgets"},
	}
}

func (w *WidgetManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "register":
		var widget api.Widget
		if err := json.Unmarshal(msg.Payload, &widget); err != nil {
			return api.Message{}, err
		}
		w.mu.Lock()
		w.widgets[widget.ID] = widget
		w.mu.Unlock()
		w.save()

		// Publish event
		if w.router != nil {
			widgetData, _ := json.Marshal(widget)
			publishReq, _ := json.Marshal(map[string]any{
				"topic": "dashboard:widget-registered",
				"data":  json.RawMessage(widgetData),
			})

			w.router(ctx, api.Message{
				ID:      "evt-widget-reg-" + widget.ID,
				Type:    api.TypeEvent,
				Sender:  w.ID(),
				Target:  "events",
				Method:  "publish",
				Payload: publishReq,
			})
		}
		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  w.ID(),
			Target:  msg.Sender,
			Payload: []byte(`{"status":"ok"}`),
		}, nil

	case "unregister":
		var req struct{ ID string }
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}
		w.mu.Lock()
		delete(w.widgets, req.ID)
		w.mu.Unlock()
		w.save()

		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  w.ID(),
			Target:  msg.Sender,
			Payload: []byte(`{"status":"ok"}`),
		}, nil

	case "update":
		var update api.WidgetUpdate
		if err := json.Unmarshal(msg.Payload, &update); err != nil {
			return api.Message{}, err
		}
		w.mu.Lock()
		widget, ok := w.widgets[update.ID]
		if ok {
			widget.Content = update.Content
			w.widgets[update.ID] = widget
		}
		w.mu.Unlock()
		if ok {
			w.save()
		}

		if ok && w.router != nil {
			// Broadcast to all frontends
			w.router(ctx, api.Message{
				ID:      "evt-widget-upd-" + update.ID,
				Type:    api.TypeEvent,
				Sender:  w.ID(),
				Target:  "*", // Broadcast
				Method:  "dashboard:widget-updated",
				Payload: msg.Payload,
				Metadata: map[string]any{
					"widget_id": update.ID,
				},
			})
		}
		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  w.ID(),
			Target:  msg.Sender,
			Payload: []byte(`{"status":"ok"}`),
		}, nil

	case "list":
		w.mu.RLock()
		defer w.mu.RUnlock()
		widgets := make([]api.Widget, 0, len(w.widgets))
		for _, v := range w.widgets {
			widgets = append(widgets, v)
		}
		payload, _ := json.Marshal(widgets)
		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  w.ID(),
			Target:  msg.Sender,
			Payload: payload,
		}, nil
	}
	return api.Message{}, nil
}

func (w *WidgetManager) Shutdown(ctx context.Context) error {
	w.save()
	return nil
}

func (w *WidgetManager) save() {
	if w.store == nil {
		return
	}
	w.mu.RLock()
	data, _ := json.Marshal(w.widgets)
	w.mu.RUnlock()
	_ = w.store.Set("system", "widgets", data)
}

func (w *WidgetManager) load() {
	if w.store == nil {
		return
	}
	data, err := w.store.Get("system", "widgets")
	if err == nil && data != nil {
		w.mu.Lock()
		_ = json.Unmarshal(data, &w.widgets)
		w.mu.Unlock()
	}
}

func (w *WidgetManager) ListWidgets() []api.Widget {
	w.mu.RLock()
	defer w.mu.RUnlock()
	res := make([]api.Widget, 0, len(w.widgets))
	for _, v := range w.widgets {
		res = append(res, v)
	}
	return res
}

// CommandManager maintains a registry of all system-wide actions and component capabilities.
type CommandManager struct {
	mu       sync.RWMutex
	registry map[string]api.Registration
	logger   *slog.Logger
	route    func(context.Context, api.Message)
	provider func() []api.Registration
	iam      *IdentityManager
}

func NewCommandManager(logger *slog.Logger, provider func() []api.Registration, iam *IdentityManager) *CommandManager {
	return &CommandManager{
		registry: make(map[string]api.Registration),
		logger:   logger,
		provider: provider,
		iam:      iam,
	}
}

func (c *CommandManager) SetRouter(r func(context.Context, api.Message)) {
	c.route = r
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
		var allTargets []api.Registration
		if c.provider != nil {
			allTargets = c.provider()
		} else {
			c.mu.RLock()
			for _, reg := range c.registry {
				allTargets = append(allTargets, reg)
			}
			c.mu.RUnlock()
		}

		// Filter targets based on Identity
		var filtered []api.Registration
		actor := msg.Actor
		if actor == "" {
			actor = msg.Sender
		}

		// Context-relative discovery
		var contextID string
		if msg.Metadata != nil {
			if ctxID, ok := msg.Metadata["context"].(string); ok {
				contextID = ctxID
			}
		}

		for _, reg := range allTargets {
			// If registration ID itself matches a capability (lazy check)
			// But usually we filter individual capabilities
			var allowedCaps []api.Capability
			for _, cap := range reg.Capabilities {
				if c.iam != nil {
					// Check if allowed for actor
					if c.iam.AuthorizeWithContext(actor, reg.ID, cap.Method, contextID) {
						allowedCaps = append(allowedCaps, cap)
					}
				} else {
					allowedCaps = append(allowedCaps, cap)
				}
			}

			// Only add registrations that have at least one allowed capability
			// Or ones that represent frontends (no capabilities listed)
			if len(allowedCaps) > 0 || (reg.Type == "frontend" && len(reg.Capabilities) == 0) {
				filtered = append(filtered, api.Registration{
					ID:           reg.ID,
					Type:         reg.Type,
					Status:       reg.Status,
					Capabilities: allowedCaps,
				})
			}
		}

		payload, _ := json.Marshal(map[string]any{
			"targets": filtered,
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
