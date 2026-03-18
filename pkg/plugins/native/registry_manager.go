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

type RouterSetter interface {
	SetRouter(r func(context.Context, api.Message))
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

		loadedPlugins := make([]api.Plugin, 0)

		for _, p := range req.Plugins {
			plugin, err := r.loadPluginInstance(ctx, p)
			if err != nil {
				r.logger.Error("failed to load plugin during provision", "id", p.ID, "error", err)
				continue
			}
			if plugin != nil {
				loadedPlugins = append(loadedPlugins, plugin)
			}
		}

		// Now that all plugins (including command-manager) are registered in the kernel,
		// we can register their capabilities with the command manager.
		for _, p := range loadedPlugins {
			caps := p.Capabilities()
			if caps == nil {
				caps = []api.Capability{}
			}
			capsData, _ := json.Marshal(caps)
			r.kernel.RouteMessage(ctx, api.Message{
				ID:        "reg-cm-" + p.ID(),
				Type:      api.TypeRequest,
				Sender:    r.ID(),
				Target:    "plugin-command-manager",
				Method:    "register",
				Payload:   []byte(`{"id":"` + p.ID() + `","type":"plugin","capabilities":` + string(capsData) + `}`),
				Timestamp: time.Now().Unix(),
			})
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
		plugin, err := r.loadPluginInstance(ctx, p)
		if err != nil {
			return api.Message{}, err
		}
		if plugin != nil {
			// Cross-register
			caps := plugin.Capabilities()
			if caps == nil {
				caps = []api.Capability{}
			}
			capsData, _ := json.Marshal(caps)
			r.kernel.RouteMessage(ctx, api.Message{
				ID:      "reg-cm-" + plugin.ID(),
				Type:    api.TypeRequest,
				Sender:  r.ID(),
				Target:  "plugin-command-manager",
				Method:  "register",
				Payload: []byte(`{"id":"` + plugin.ID() + `","type":"plugin","capabilities":` + string(capsData) + `}`),
			})
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

func (r *RegistryManager) loadPluginInstance(ctx context.Context, def PluginDef) (api.Plugin, error) {
	r.logger.Info("loading plugin", "id", def.ID, "type", def.Type)

	switch def.Type {
	case "native":
		factory, ok := Registry[def.ID]
		if !ok {
			return nil, fmt.Errorf("native plugin not found in registry: %s", def.ID)
		}
		p, err := factory(ctx, r.logger, r.state)
		if err != nil {
			return nil, err
		}
		
		// Use interface instead of type-checking for every plugin
		if setter, ok := p.(RouterSetter); ok {
			setter.SetRouter(r.kernel.RouteMessage)
		}

		plugin := p.(api.Plugin)
		r.kernel.RegisterPlugin(plugin)
		return plugin, nil

	case "wasm":
		if r.wasm == nil {
			return nil, fmt.Errorf("wasm runtime not available")
		}
		content, err := os.ReadFile(def.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read wasm file: %w", err)
		}

		// Asynchronously load and register WASM to avoid blocking provisioning or the message bus
		go func() {
			r.logger.Info("starting async wasm load", "id", def.ID)
			p, err := r.wasm.LoadPlugin(context.Background(), def.ID, content, def.MemoryLimit, def.FuelLimit)
			if err != nil {
				r.logger.Error("failed to load wasm plugin", "id", def.ID, "error", err)
				return
			}
			r.kernel.RegisterPlugin(p)
			r.logger.Info("wasm plugin registered in kernel", "id", def.ID)

			// Register with command manager
			caps := p.Capabilities()
			if caps == nil {
				caps = []api.Capability{}
			}
			capsData, _ := json.Marshal(caps)
			r.kernel.RouteMessage(context.Background(), api.Message{
				ID:        "reg-cm-" + p.ID(),
				Type:      api.TypeRequest,
				Sender:    r.ID(),
				Target:    "plugin-command-manager",
				Method:    "register",
				Payload:   []byte(`{"id":"` + p.ID() + `","type":"plugin","capabilities":` + string(capsData) + `}`),
				Timestamp: time.Now().Unix(),
			})
		}()

		return nil, nil // Return nil because registration happens asynchronously

	default:
		return nil, fmt.Errorf("unknown plugin type: %s", def.Type)
	}
}

func (r *RegistryManager) Shutdown(ctx context.Context) error { return nil }
