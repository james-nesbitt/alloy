package native

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
)

// KernelInternal interface to avoid circular dependency.
type KernelInternal interface {
	RegisterPlugin(p api.Plugin)
	RegisterMetadata(info api.PluginMetadata, loader api.PluginLoader)
	RouteMessage(ctx context.Context, msg api.Message)
}

type RouterSetter interface {
	SetRouter(r func(context.Context, api.Message))
}

type RegistryManager struct {
	logger *slog.Logger
	kernel KernelInternal
	state  storage.StateStore
}

func NewRegistryManager(logger *slog.Logger, k KernelInternal, state storage.StateStore) *RegistryManager {
	return &RegistryManager{
		logger: logger,
		kernel: k,
		state:  state,
	}
}

func (r *RegistryManager) ID() string { return "plugin-registry" }

func (r *RegistryManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "register", Description: "Register a list of plugins for later loading"},
		{Method: "provision", Description: "Batch load plugins from manifest"},
	}
}

type ProvisionRequest struct {
	Plugins []PluginDef `json:"plugins"`
}

type PluginDef struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"` // "native" or "wasm"
	Path         string           `json:"path,omitempty"`
	MemoryLimit  uint64           `json:"memory_limit_mb,omitempty"`
	FuelLimit    uint64           `json:"fuel_limit,omitempty"`
	LoadTime     api.PluginLoadTime `json:"load_time,omitempty"`
	Capabilities []api.Capability `json:"capabilities,omitempty"`
}

func (r *RegistryManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "register":
		var req ProvisionRequest
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		for _, p := range req.Plugins {
			r.delegateRegister(ctx, p)
		}

		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    r.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"registration_triggered"}`),
			Timestamp: time.Now().Unix(),
		}, nil

	case "provision":
		var req ProvisionRequest
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		for _, p := range req.Plugins {
			r.delegateLoad(ctx, p)
		}

		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    r.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"provisioning_triggered"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	}
	return api.Message{}, nil
}

func (r *RegistryManager) delegateRegister(ctx context.Context, def PluginDef) {
	r.logger.Info("registering plugin metadata", "id", def.ID, "type", def.Type, "load_time", def.LoadTime)

	if def.Type == "native" {
		r.kernel.RegisterMetadata(api.PluginMetadata{
			ID:           def.ID,
			LoadTime:     def.LoadTime,
			Capabilities: def.Capabilities,
		}, r)
		return
	}

	if def.Type == "wasm" {
		payload, _ := json.Marshal(def)
		r.kernel.RouteMessage(ctx, api.Message{
			ID:      "reg-wasm-" + def.ID,
			Sender:  r.ID(),
			Actor:   "system",
			Target:  "plugin-wasm-manager",
			Method:  "register",
			Payload: payload,
		})
	}
}

func (r *RegistryManager) LoadPlugin(ctx context.Context, id string) (api.Plugin, error) {
	factory, ok := Registry[id]
	if !ok {
		return nil, fmt.Errorf("native plugin factory not found: %s", id)
	}
	instance, err := factory(ctx, r.logger, r.state)
	if err != nil {
		return nil, err
	}
	p, ok := instance.(api.Plugin)
	if !ok {
		return nil, fmt.Errorf("plugin %s does not implement api.Plugin", id)
	}

	if s, ok := instance.(RouterSetter); ok {
		s.SetRouter(r.kernel.RouteMessage)
	}

	// Wait for plugin readiness if implemented
	if ready, ok := instance.(api.ReadinessProvider); ok {
		r.logger.Info("waiting for native plugin readiness", "plugin_id", id)
		if err := ready.Ready(ctx); err != nil {
			return nil, fmt.Errorf("native plugin %s failed readiness check: %w", id, err)
		}
	}

	// Native registration with CM
	caps := p.Capabilities()
	if caps == nil {
		caps = []api.Capability{}
	}
	capsData, _ := json.Marshal(caps)
	r.kernel.RouteMessage(ctx, api.Message{
		ID:      "reg-cm-" + p.ID(),
		Sender:  r.ID(),
		Actor:   "system",
		Target:  "plugin-command-manager",
		Method:  "register",
		Payload: []byte(`{"id":"` + p.ID() + `","type":"native","capabilities":` + string(capsData) + `}`),
	})

	return p, nil
}

func (r *RegistryManager) delegateLoad(ctx context.Context, def PluginDef) {
	r.logger.Info("triggering plugin load", "id", def.ID, "type", def.Type)

	if def.Type == "native" {
		p, err := r.LoadPlugin(ctx, def.ID)
		if err != nil {
			r.logger.Error("failed to load native plugin", "id", def.ID, "error", err)
			return
		}
		r.kernel.RegisterPlugin(p)
		return
	}

	if def.Type == "wasm" {
		payload, _ := json.Marshal(def)
		r.kernel.RouteMessage(ctx, api.Message{
			ID:      "load-wasm-" + def.ID,
			Sender:  r.ID(),
			Actor:   "system",
			Target:  "plugin-wasm-manager",
			Method:  "load",
			Payload: payload,
		})
	}
}

func (r *RegistryManager) Shutdown(ctx context.Context) error { return nil }
