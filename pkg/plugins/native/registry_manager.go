package native

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
)

// KernelInternal interface to avoid circular dependency.
type KernelInternal interface {
	RegisterPlugin(p api.Plugin)
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
		{Method: "provision", Description: "Batch load plugins from manifest"},
	}
}

type ProvisionRequest struct {
	Plugins []PluginDef `json:"plugins"`
}

type PluginDef struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // "native" or "wasm"
	Path        string `json:"path,omitempty"`
	MemoryLimit uint64 `json:"memory_limit_mb,omitempty"`
	FuelLimit   uint64 `json:"fuel_limit,omitempty"`
}

func (r *RegistryManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
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

func (r *RegistryManager) delegateLoad(ctx context.Context, def PluginDef) {
	r.logger.Info("triggering plugin load", "id", def.ID, "type", def.Type)

	if def.Type == "native" {
		factory, ok := Registry[def.ID]
		if !ok {
			r.logger.Error("native plugin not found", "id", def.ID)
			return
		}
		instance, _ := factory(ctx, r.logger, r.state)
		if i, ok := instance.(api.Plugin); ok {
			r.kernel.RegisterPlugin(i)
			if s, ok := instance.(RouterSetter); ok {
				s.SetRouter(r.kernel.RouteMessage)
			}
			
			// Native registration with CM
			caps := i.Capabilities()
			if caps == nil { caps = []api.Capability{} }
			capsData, _ := json.Marshal(caps)
			r.kernel.RouteMessage(ctx, api.Message{
				Target: "plugin-command-manager",
				Method: "register",
				Payload: []byte(`{"id":"` + i.ID() + `","type":"native","capabilities":` + string(capsData) + `}`),
			})
		}
		return
	}

	if def.Type == "wasm" {
		payload, _ := json.Marshal(def)
		r.kernel.RouteMessage(ctx, api.Message{
			ID:      "load-wasm-" + def.ID,
			Sender:  r.ID(),
			Target:  "plugin-wasm-manager",
			Method:  "load",
			Payload: payload,
		})
	}
}

func (r *RegistryManager) Shutdown(ctx context.Context) error { return nil }
