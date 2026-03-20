package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

// KernelInternal interface to avoid circular dependency.
type KernelInternal interface {
	RegisterPlugin(p api.Plugin)
	RouteMessage(ctx context.Context, msg api.Message)
}

// Manager is an Alloy native plugin that manages WASM plugin lifecycles.
type Manager struct {
	logger  *slog.Logger
	runtime *Runtime
	kernel  KernelInternal
	
	mu      sync.Mutex
	defs    map[string]pluginDef
	plugins map[string]*Instance
}

func NewManager(logger *slog.Logger, runtime *Runtime, k KernelInternal) *Manager {
	return &Manager{
		logger:  logger,
		runtime: runtime,
		kernel:  k,
		defs:    make(map[string]pluginDef),
		plugins: make(map[string]*Instance),
	}
}

func (m *Manager) ID() string { return "plugin-wasm-manager" }

func (m *Manager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "load", Description: "Load a WASM plugin"},
		{Method: "reload", Description: "Reload an already loaded WASM plugin from its original path"},
		{Method: "status", Description: "Get status of all WASM plugins"},
	}
}

type pluginDef struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	MemoryLimit uint64 `json:"memory_limit_mb,omitempty"`
	FuelLimit   uint64 `json:"fuel_limit,omitempty"`
}

func (m *Manager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "load":
		var def pluginDef
		if err := json.Unmarshal(msg.Payload, &def); err != nil {
			return api.Message{}, err
		}
		go m.loadAndRegister(def.ID, def.Path, def.MemoryLimit, def.FuelLimit)
		return m.ack(msg.ID, msg.Sender, "loading"), nil

	case "reload":
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}
		go m.reloadPlugin(req.ID)
		return m.ack(msg.ID, msg.Sender, "reloading"), nil

	case "status":
		m.mu.Lock()
		defer m.mu.Unlock()
		statusList := make([]map[string]any, 0, len(m.plugins))
		for id, p := range m.plugins {
			st, errStr := p.Status()
			statusList = append(statusList, map[string]any{
				"id":    id,
				"status": string(st),
				"error":  errStr,
			})
		}
		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  m.ID(),
			Target:  msg.Sender,
			Payload: mustMarshal(statusList),
		}, nil
	}
	return api.Message{}, nil
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func (m *Manager) ack(id, target, status string) api.Message {
	return api.Message{
		ID:      id + "-resp",
		Type:    api.TypeResponse,
		Sender:  m.ID(),
		Target:  target,
		Payload: []byte(`{"status":"` + status + `"}`),
	}
}

func (m *Manager) loadAndRegister(id, path string, mem, fuel uint64) {
	m.mu.Lock()
	m.defs[id] = pluginDef{ID: id, Path: path, MemoryLimit: mem, FuelLimit: fuel}
	m.mu.Unlock()

	m.logger.Info("loading wasm plugin", "id", id, "path", path)
	content, err := os.ReadFile(path)
	if err != nil {
		m.logger.Error("failed to read wasm file", "id", id, "error", err)
		m.publishError("plugin:load_failed", id, err.Error())
		return
	}

	p, err := m.runtime.LoadPlugin(context.Background(), id, content, mem, fuel)
	if err != nil {
		m.logger.Error("failed to instantiate wasm plugin", "id", id, "error", err)
		m.publishError("plugin:load_failed", id, err.Error())
		return
	}

	instance, ok := p.(*Instance)
	if ok {
		m.mu.Lock()
		m.plugins[id] = instance
		m.mu.Unlock()
	}

	m.kernel.RegisterPlugin(p)
	m.logger.Info("wasm plugin registered", "id", id)

	// Register with Command Manager
	m.registerWithCommandManager(id, p.Capabilities())
}

func (m *Manager) reloadPlugin(id string) {
	m.mu.Lock()
	def, ok := m.defs[id]
	m.mu.Unlock()

	if !ok {
		m.logger.Error("failed to reload plugin: definition not found", "id", id)
		m.publishError("plugin:reload_failed", id, "definition not found")
		return
	}

	m.logger.Info("reloading wasm plugin", "id", id, "path", def.Path)
	content, err := os.ReadFile(def.Path)
	if err != nil {
		m.logger.Error("failed to read wasm file for reload", "id", id, "error", err)
		m.publishError("plugin:reload_failed", id, err.Error())
		return
	}

	// Runtime swap (this closes the old module instance)
	p, err := m.runtime.LoadPlugin(context.Background(), id, content, def.MemoryLimit, def.FuelLimit)
	if err != nil {
		m.logger.Error("failed to swap wasm plugin on reload", "id", id, "error", err)
		m.publishError("plugin:reload_failed", id, err.Error())
		return
	}

	instance, ok := p.(*Instance)
	if ok {
		m.mu.Lock()
		m.plugins[id] = instance
		m.mu.Unlock()
	}

	// Update the kernel registry (hot swap)
	m.kernel.RegisterPlugin(p)
	m.logger.Info("wasm plugin reloaded successfully", "id", id)

	// Update Command Manager
	m.registerWithCommandManager(id, p.Capabilities())
}

func (m *Manager) registerWithCommandManager(id string, caps []api.Capability) {
	if caps == nil {
		caps = []api.Capability{}
	}
	capsData, _ := json.Marshal(caps)
	m.kernel.RouteMessage(context.Background(), api.Message{
		ID:      "reg-cm-" + id + "-" + fmt.Sprint(time.Now().UnixNano()),
		Type:    api.TypeRequest,
		Sender:  m.ID(),
		Target:  "plugin-command-manager",
		Method:  "register",
		Payload: []byte(`{"id":"` + id + `","type":"wasm","capabilities":` + string(capsData) + `}`),
		Timestamp: time.Now().Unix(),
	})
}

func (m *Manager) publishError(method, id, err string) {
	payload, _ := json.Marshal(map[string]string{
		"id":    id,
		"error": err,
	})
	m.kernel.RouteMessage(context.Background(), api.Message{
		ID:        "evt-err-" + id + "-" + fmt.Sprint(time.Now().UnixNano()),
		Type:      "event",
		Sender:    m.ID(),
		Target:    "plugin-events",
		Method:    method,
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	})
}

func (m *Manager) Shutdown(ctx context.Context) error {
	return m.runtime.Close(ctx)
}
