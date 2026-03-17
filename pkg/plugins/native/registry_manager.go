package native

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
)

// KernelInternal interface to avoid circular dependency if needed, 
// but for native plugins we can often just use the concrete type or a subset interface.
type KernelInternal interface {
	RegisterPlugin(p api.Plugin)
	RouteMessage(ctx context.Context, msg api.Message)
}

// Note: I used api.Plugin because it might be better to move the interface to api/
// But for now it resides in kernel. Let's stick with that for now or move it.
// Checking pkg/kernel/kernel.go ... yes it is there.

type RegistryManager struct {
	logger  *slog.Logger
	kernel  KernelInternal
	state   storage.StateStore
	wasm    WasmLoader
}

type WasmLoader interface {
	LoadPlugin(ctx context.Context, id string, wasm []byte, memoryLimitMB uint64, fuelLimit uint64) (api.Plugin, error)
}

func NewRegistryManager(logger *slog.Logger, k KernelInternal, state storage.StateStore, wasm WasmLoader) *RegistryManager {
	return &RegistryManager{
		logger: logger,
		kernel: k,
		state:  state,
		wasm:   wasm,
	}
}

func (r *RegistryManager) ID() string { return "plugin-registry" }

func (r *RegistryManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "provision", Description: "Initial batch loading of plugins"},
		{Method: "load", Description: "Load a single plugin by ID and Type"},
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
			if err := r.loadPlugin(ctx, p); err != nil {
				r.logger.Error("failed to load plugin during provision", "id", p.ID, "error", err)
			}
		}

		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    r.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"provisioned"}`),
			Timestamp: time.Now().Unix(),
		}, nil

	case "load":
		var p PluginDef
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			return api.Message{}, err
		}
		if err := r.loadPlugin(ctx, p); err != nil {
			return api.Message{}, err
		}
		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  r.ID(),
			Target:  msg.Sender,
			Payload: []byte(`{"status":"loaded"}`),
		}, nil
	}
	return api.Message{}, nil
}

func (r *RegistryManager) loadPlugin(ctx context.Context, def PluginDef) error {
	r.logger.Info("loading plugin", "id", def.ID, "type", def.Type)

	switch def.Type {
	case "native":
		factory, ok := Registry[def.ID]
		if !ok {
			return fmt.Errorf("native plugin not found in registry: %s", def.ID)
		}
		p, err := factory(ctx, r.logger, r.state)
		if err != nil {
			return err
		}
		
		// If it's the EventManager, it might need the router
		if em, ok := p.(*EventManager); ok {
			em.SetRouter(r.kernel.RouteMessage)
		}

		r.kernel.RegisterPlugin(p.(api.Plugin))
		return nil

	case "wasm":
		if r.wasm == nil {
			return fmt.Errorf("wasm runtime not available")
		}
		content, err := os.ReadFile(def.Path)
		if err != nil {
			return fmt.Errorf("failed to read wasm file: %w", err)
		}
		p, err := r.wasm.LoadPlugin(ctx, def.ID, content, def.MemoryLimit, def.FuelLimit)
		if err != nil {
			return err
		}
		r.kernel.RegisterPlugin(p)
		return nil

	default:
		return fmt.Errorf("unknown plugin type: %s", def.Type)
	}
}

func (r *RegistryManager) Shutdown(ctx context.Context) error { return nil }
