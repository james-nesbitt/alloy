package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jnesbitt/alloy-go/api"
)

// KernelInternal interface to avoid circular dependency.
type KernelInternal interface {
	RegisterPlugin(p api.Plugin)
	RegisterMetadata(info api.PluginMetadata, loader api.PluginLoader)
	RouteMessage(ctx context.Context, msg api.Message)
	Call(ctx context.Context, msg api.Message) (api.Message, error)
}

// Manager is an Alloy native plugin that manages WASM plugin lifecycles.
type Manager struct {
	logger  *slog.Logger
	runtime *Runtime
	kernel  KernelInternal
	
	mu      sync.Mutex
	defs    map[string]pluginDef
	plugins map[string]*Instance
	
	watcher *fsnotify.Watcher
	watches map[string]string // Path -> ID
}

func NewManager(logger *slog.Logger, runtime *Runtime, k KernelInternal) *Manager {
	watcher, _ := fsnotify.NewWatcher()
	m := &Manager{
		logger:  logger,
		runtime: runtime,
		kernel:  k,
		defs:    make(map[string]pluginDef),
		plugins: make(map[string]*Instance),
		watcher: watcher,
		watches: make(map[string]string),
	}
	if watcher != nil {
		go m.watchLoop()
	}
	return m
}

func (m *Manager) ID() string { return "plugin-wasm-manager" }

func (m *Manager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "register", Description: "Register a WASM plugin for later loading"},
		{Method: "load", Description: "Load a WASM plugin"},
		{Method: "reload", Description: "Reload an already loaded WASM plugin from its original path"},
		{Method: "status", Description: "Get status of all WASM plugins"},
		{Method: "watch", Description: "Enable auto-reloading for a plugin on file change"},
		{Method: "unwatch", Description: "Disable auto-reloading for a plugin"},
	}
}

type pluginDef struct {
	ID           string           `json:"id"`
	Path         string           `json:"path"`
	MemoryLimit  uint64           `json:"memory_limit_mb,omitempty"`
	FuelLimit    uint64           `json:"fuel_limit,omitempty"`
	LoadTime     api.PluginLoadTime `json:"load_time,omitempty"`
	Capabilities []api.Capability `json:"capabilities,omitempty"`
}

func (m *Manager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "register":
		var def pluginDef
		if err := json.Unmarshal(msg.Payload, &def); err != nil {
			m.logger.Error("failed to unmarshal plugin definition", "error", err)
			return api.Message{}, err
		}
		m.logger.Info("registering wasm plugin def", "id", def.ID, "path", def.Path)
		m.mu.Lock()
		m.defs[def.ID] = def
		m.mu.Unlock()

		m.kernel.RegisterMetadata(api.PluginMetadata{
			ID:           def.ID,
			LoadTime:     def.LoadTime,
			Capabilities: def.Capabilities,
		}, m)

		// Also notify command-manager of metadata
		m.kernel.RouteMessage(ctx, api.Message{
			ID:      "reg-cm-" + def.ID,
			Sender:  m.ID(),
			Actor:   "system",
			Target:  "plugin-command-manager",
			Method:  "register",
			Payload: []byte(`{"id":"` + def.ID + `","type":"wasm","status":"registered"}`),
		})

		return m.ack(msg.ID, msg.Sender, "registered"), nil

	case "load":
		var def pluginDef
		if err := json.Unmarshal(msg.Payload, &def); err != nil {
			return api.Message{}, err
		}
		go m.loadAndRegister(def.ID, def.Path, def.MemoryLimit, def.FuelLimit, def.Capabilities)
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

	case "watch":
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}
		m.mu.Lock()
		def, ok := m.defs[req.ID]
		m.mu.Unlock()
		if !ok {
			return api.Message{}, fmt.Errorf("plugin definition not found: %s", req.ID)
		}
		m.mu.Lock()
		m.watches[def.Path] = req.ID
		m.mu.Unlock()
		if m.watcher != nil {
			m.watcher.Add(def.Path)
		}
		return m.ack(msg.ID, msg.Sender, "watching"), nil

	case "unwatch":
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}
		m.mu.Lock()
		def, ok := m.defs[req.ID]
		m.mu.Unlock()
		if ok && m.watcher != nil {
			m.watcher.Remove(def.Path)
		}
		m.mu.Lock()
		delete(m.watches, def.Path)
		m.mu.Unlock()
		return m.ack(msg.ID, msg.Sender, "unwatched"), nil

	case "status":
		m.mu.Lock()
		defer m.mu.Unlock()
		statusList := make([]map[string]any, 0, len(m.defs))
		for id, def := range m.defs {
			if p, active := m.plugins[id]; active {
				st, errStr := p.Status()
				statusList = append(statusList, map[string]any{
					"id":     id,
					"status": string(st),
					"error":  errStr,
					"path":   def.Path,
				})
			} else {
				statusList = append(statusList, map[string]any{
					"id":     id,
					"status": "dormant",
					"path":   def.Path,
				})
			}
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

func (m *Manager) LoadPlugin(ctx context.Context, id string) (api.Plugin, error) {
	m.mu.Lock()
	def, ok := m.defs[id]
	m.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("plugin definition not found: %s", id)
	}

	m.logger.Info("loading wasm plugin", "id", id, "path", def.Path)
	content, err := os.ReadFile(def.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read wasm file: %w", err)
	}

	p, err := m.runtime.LoadPlugin(ctx, id, content, def.MemoryLimit, def.FuelLimit, def.Capabilities)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate wasm plugin: %w", err)
	}

	instance, ok := p.(*Instance)
	if ok {
		m.plugins[id] = instance
	}

	// Note: We don't call registerWithCommandManager here anymore because 
	// Kernel.RegisterPlugin now emits a component:registered event with 
	// actual capabilities once the plugin is active.

	return p, nil
}

func (m *Manager) loadAndRegister(id, path string, mem, fuel uint64, caps []api.Capability) {
	m.mu.Lock()
	m.defs[id] = pluginDef{ID: id, Path: path, MemoryLimit: mem, FuelLimit: fuel, Capabilities: caps}
	m.mu.Unlock()

	// Initial loading status
	m.kernel.RouteMessage(context.Background(), api.Message{
		ID:      "load-cm-" + id,
		Sender:  m.ID(),
		Actor:   "system",
		Target:  "plugin-command-manager",
		Method:  "register",
		Payload: []byte(`{"id":"` + id + `","type":"wasm","status":"loading"}`),
	})

	p, err := m.LoadPlugin(context.Background(), id)
	if err != nil {
		m.logger.Error("failed to load and register plugin", "id", id, "error", err)
		m.kernel.RouteMessage(context.Background(), api.Message{
			ID:      "error-cm-" + id,
			Sender:  m.ID(),
			Actor:   "system",
			Target:  "plugin-command-manager",
			Method:  "register",
			Payload: []byte(`{"id":"` + id + `","type":"wasm","status":"error"}`),
		})
		m.publishError("plugin:load_failed", id, err.Error())
		return
	}

	m.kernel.RegisterPlugin(p)

	// Finally active
	m.kernel.RouteMessage(context.Background(), api.Message{
		ID:      "active-cm-" + id,
		Sender:  m.ID(),
		Actor:   "system",
		Target:  "plugin-command-manager",
		Method:  "register",
		Payload: []byte(`{"id":"` + id + `","type":"wasm","status":"active"}`),
	})

	m.logger.Info("wasm plugin registered", "id", id)
}

func (m *Manager) watchLoop() {
	if m.watcher == nil {
		return
	}
	for {
		select {
		case event, ok := <-m.watcher.Events:
			if !ok { return }
			if event.Op&fsnotify.Write == fsnotify.Write {
				m.mu.Lock()
				id, ok := m.watches[event.Name]
				m.mu.Unlock()
				if ok {
					m.logger.Info("auto-reloading plugin due to file change", "id", id, "path", event.Name)
					m.reloadPlugin(id)
				}
			}
		case err, ok := <-m.watcher.Errors:
			if !ok { return }
			m.logger.Error("watcher error", "error", err)
		}
	}
}

func (m *Manager) reloadPlugin(id string) {
	m.mu.Lock()
	def, ok := m.defs[id]
	oldInstance, hasOld := m.plugins[id]
	m.mu.Unlock()

	if !ok {
		m.logger.Error("failed to reload plugin: definition not found", "id", id)
		m.publishError("plugin:reload_failed", id, "definition not found")
		return
	}

	// 1. Save state if supported
	var state []byte
	if hasOld {
		m.logger.Info("attempting to save state for plugin", "id", id)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		resp, err := oldInstance.HandleMessage(ctx, api.Message{
			ID:     "reload-save-" + id,
			Type:   api.TypeRequest,
			Sender: m.ID(),
			Target: id,
			Method: "system:save_state",
		})
		cancel()
		if err == nil && len(resp.Payload) > 0 {
			state = resp.Payload
			m.logger.Info("state saved successfully", "id", id, "size", len(state))
		}
	}

	m.logger.Info("reloading wasm plugin", "id", id, "path", def.Path)
	content, err := os.ReadFile(def.Path)
	if err != nil {
		m.logger.Error("failed to read wasm file for reload", "id", id, "error", err)
		m.publishError("plugin:reload_failed", id, err.Error())
		return
	}

	// 2. Clear old instance and names to avoid conflicts
	if hasOld {
		_ = oldInstance.Shutdown(context.Background())
		m.mu.Lock()
		delete(m.plugins, id)
		m.mu.Unlock()
	}

	// 3. Load new plugin
	p, err := m.runtime.LoadPlugin(context.Background(), id, content, def.MemoryLimit, def.FuelLimit, def.Capabilities)
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

		// 4. Restore state if we have it
		if len(state) > 0 {
			m.logger.Info("restoring state for plugin", "id", id)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_, _ = instance.HandleMessage(ctx, api.Message{
				ID:      "reload-load-" + id,
				Type:    api.TypeRequest,
				Sender:  m.ID(),
				Target:  id,
				Method:  "system:load_state",
				Payload: state,
			})
			cancel()
		}
	}

	// Update the kernel registry (hot swap)
	m.kernel.RegisterPlugin(p)
	m.logger.Info("wasm plugin reloaded successfully", "id", id)
}

func (m *Manager) publishError(method, id, err string) {
	payload, _ := json.Marshal(map[string]string{
		"id":    id,
		"error": err,
	})
	m.kernel.RouteMessage(context.Background(), api.Message{
		ID:        "evt-err-" + id + "-" + fmt.Sprint(time.Now().UnixNano()),
		Type:      api.TypeEvent,
		Sender:    m.ID(),
		Actor:     "system",
		Target:    "plugin-events",
		Method:    method,
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	})
}

func (m *Manager) Shutdown(ctx context.Context) error {
	return m.runtime.Close(ctx)
}
