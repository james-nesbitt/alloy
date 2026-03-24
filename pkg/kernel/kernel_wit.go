package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/plugins/native"
	"github.com/james-nesbitt/alloy/pkg/storage"
	"github.com/james-nesbitt/alloy/pkg/wasm"
)

// WITKernel is a kernel implementation that uses the WIT-based WASM runtime.
type WITKernel struct {
	logger       *slog.Logger
	mu           sync.RWMutex
	plugins      map[string]api.Plugin
	metadata     map[string]api.PluginMetadata
	loaders      map[string]api.PluginLoader
	frontends    map[string]chan<- api.Message
	interceptors []api.Interceptor
	loading      map[string]chan struct{}
	stopCh       chan struct{}
	stopOnce     sync.Once
	wasmManager  *wasm.Manager
	storage      storage.StateStore
	dataDir      string

	telemetry *Telemetry

	// Internal core services
	events   api.Plugin
	commands api.Plugin

	// Capability mapping
	capabilityMap map[string]string // capability name -> plugin ID
}

// NewWITKernel creates a new WIT-based kernel.
func NewWITKernel(
	logger *slog.Logger,
	storage storage.StateStore,
	dataDir string,
	metricsAddr string,
) (*WITKernel, error) {
	tel, _ := initTelemetry(metricsAddr)

	// Create the kernel
	kernel := &WITKernel{
		logger:    logger,
		plugins:   make(map[string]api.Plugin),
		metadata:  make(map[string]api.PluginMetadata),
		loaders:   make(map[string]api.PluginLoader),
		frontends: make(map[string]chan<- api.Message),
		loading:   make(map[string]chan struct{}),
		stopCh:    make(chan struct{}),
		storage:   storage,
		dataDir:   dataDir,
		capabilityMap: make(map[string]string),
		telemetry:     tel,
	}

	// Create the WASM manager
	router := func(ctx context.Context, msg api.Message) {
		kernel.RouteMessage(ctx, msg)
	}

	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		return kernel.handleMessageSync(ctx, msg)
	}

	wasmManager, err := wasm.NewManager(logger, storage, dataDir, router, call)
	if err != nil {
		return nil, err
	}

	kernel.wasmManager = wasmManager

	// Initialize and register core internal plugins
	kernel.events = native.NewEventManager(logger)
	kernel.RegisterPlugin(kernel.events)

	kernel.commands = native.NewCommandManager(logger, func() []api.Registration {
		kernel.mu.RLock()
		defer kernel.mu.RUnlock()
		var regs []api.Registration
		// Convert metadata to registrations
		for id, meta := range kernel.metadata {
			regs = append(regs, api.Registration{
				ID:           id,
				Type:         "plugin",
				Status:       "active",
				Capabilities: meta.Capabilities,
			})
		}
		// Also include internal plugins
		for id, plugin := range kernel.plugins {
			if _, ok := kernel.metadata[id]; !ok {
				regs = append(regs, api.Registration{
					ID:           id,
					Type:         "plugin",
					Status:       "active",
					Capabilities: plugin.Capabilities(),
				})
			}
		}
		return regs
	})
	kernel.RegisterPlugin(kernel.commands)

	// Back-fill existing registrations into the command manager
	for _, md := range kernel.metadata {
		reg := struct {
			ID           string           `json:"id"`
			Type         string           `json:"type"`
			Status       string           `json:"status"`
			Capabilities []api.Capability `json:"capabilities"`
		}{
			ID:           md.ID,
			Type:         "plugin",
			Status:       "active",
			Capabilities: md.Capabilities,
		}
		payload, _ := json.Marshal(reg)
		kernel.commands.HandleMessage(context.Background(), api.Message{
			ID:      "backfill-" + md.ID,
			Sender:  "kernel",
			Target:  "command-manager",
			Method:  "register",
			Payload: payload,
		})
	}

	// Initialize and register security interceptor
	iamInterceptor := NewIAMInterceptor(kernel)
	kernel.RegisterInterceptor(iamInterceptor)

	// Register WASM manager as a plugin so it can be called by others (e.g. for loading/hot-reload)
	kernel.RegisterPlugin(&wasmManagerPlugin{kernel: kernel})

	// Start the monitor
	kernel.wasmManager.StartMonitor(context.Background(), 30*time.Second)

	return kernel, nil
}

// routeMessage routes messages to the appropriate destination.
func (k *WITKernel) RouteMessage(ctx context.Context, msg api.Message) {
	k.logger.Debug("kernel routing message", "id", msg.ID, "sender", msg.Sender, "target", msg.Target, "method", msg.Method)
	k.telemetry.RecordMessage(ctx, msg.Sender, msg.Target, msg.Method)

	// Apply interceptors
	k.mu.RLock()
	interceptors := make([]api.Interceptor, len(k.interceptors))
	copy(interceptors, k.interceptors)
	k.mu.RUnlock()

	for _, interceptor := range interceptors {
		var cont bool
		var err error
		msg, cont, err = interceptor.PreRoute(ctx, msg)
		if err != nil {
			k.logger.Error("interceptor failed", "error", err)
			return
		}
		if !cont {
			return
		}
	}

	// Route to frontends if this is a broadcast or event
	if msg.Target == "" || msg.Target == "*" {
		k.mu.RLock()
		frontends := make(map[string]chan<- api.Message, len(k.frontends))
		for id, ch := range k.frontends {
			frontends[id] = ch
		}
		k.mu.RUnlock()

		for connID, ch := range frontends {
			select {
			case ch <- msg:
			case <-ctx.Done():
				return
			case <-k.stopCh:
				return
			default:
				k.logger.Warn("frontend channel full", "connID", connID)
			}
		}
		return
	}

	// Route to a specific plugin or frontend
	if msg.Target == "kernel" || msg.Target == "system" {
		k.handleInternalMessage(ctx, msg)
		return
	}

	// Update capability map if this is a registration message
	if msg.Target == "command-manager" {
		if msg.Method == "register" {
			var reg api.Registration
			if err := json.Unmarshal(msg.Payload, &reg); err == nil {
				id := reg.ID
				if id == "" {
					id = msg.Sender
				}
				k.mu.Lock()
				for _, cap := range reg.Capabilities {
					if cap.Method != "" {
						k.capabilityMap[cap.Method] = id
					}
				}
				k.mu.Unlock()
			}
		} else if msg.Method == "register-capability" {
			var cap api.Capability
			if err := json.Unmarshal(msg.Payload, &cap); err == nil {
				if cap.Method != "" {
					id := msg.Sender
					k.mu.Lock()
					k.capabilityMap[cap.Method] = id
					// Update metadata as well
					if md, ok := k.metadata[id]; ok {
						found := false
						for _, existing := range md.Capabilities {
							if existing.Method == cap.Method {
								found = true
								break
							}
						}
						if !found {
							md.Capabilities = append(md.Capabilities, cap)
							k.metadata[id] = md
						}
					}
					k.mu.Unlock()
				}
			}
		}
	}

	k.logger.Debug("routing to specific target", "target", msg.Target, "msgID", msg.ID)

	k.mu.RLock()
	ch, isFrontend := k.frontends[msg.Target]
	plugin, isPlugin := k.plugins[msg.Target]
	_, isLazy := k.metadata[msg.Target]
	capabilityTarget, isCapability := k.capabilityMap[msg.Target]
	k.mu.RUnlock()

	if isCapability && !isPlugin && !isFrontend {
		k.logger.Debug("resolved capability to target", "capability", msg.Target, "target", capabilityTarget)
		msg.Target = capabilityTarget
		// Re-fetch target after resolution
		k.mu.RLock()
		ch, isFrontend = k.frontends[msg.Target]
		plugin, isPlugin = k.plugins[msg.Target]
		_, isLazy = k.metadata[msg.Target]
		k.mu.RUnlock()
	}

	if isFrontend {
		select {
		case ch <- msg:
		default:
			k.logger.Warn("frontend channel full", "target", msg.Target)
		}
		return
	}

	if isPlugin {
		// Asynchronous routing
		go func() {
			k.logger.Debug("background handling message for plugin", "id", msg.Target)
			resp, err := plugin.HandleMessage(ctx, msg)
			k.logger.Debug("plugin handle returned", "id", msg.Target, "respID", resp.ID)
			if err != nil {
				k.logger.Error("plugin handle failed", "id", msg.Target, "error", err)

				// Report failure back to sender if it was a request
				if msg.Type == api.TypeRequest || msg.Type == "" {
					resp := api.Message{
						ID:        msg.ID + "-resp",
						Type:      api.TypeResponse,
						Sender:    "system",
						Target:    msg.Sender,
						Payload:   []byte(fmt.Sprintf(`{"error": %q}`, err.Error())),
						Timestamp: time.Now().Unix(),
					}
					k.RouteMessage(ctx, resp)
				}
				return
			}
			if resp.ID != "" && resp.Target != "" {
				k.RouteMessage(ctx, resp)
			}
		}()
		return
	}

	if isLazy {
		// Target is a lazy-loaded plugin
		k.logger.Debug("triggering lazy-load for plugin", "id", msg.Target)
		if err := k.lazyLoadPlugin(ctx, msg.Target); err == nil {
			k.mu.RLock()
			plugin, ok := k.plugins[msg.Target]
			k.mu.RUnlock()
			if ok {
				go func() {
					k.logger.Debug("background handling message for lazy-loaded plugin", "id", msg.Target)
					resp, err := plugin.HandleMessage(ctx, msg)
					if err != nil {
						k.logger.Error("lazy-loaded plugin handle failed", "id", msg.Target, "error", err)
						return
					}
					if resp.ID != "" && resp.Target != "" {
						k.RouteMessage(ctx, resp)
					}
				}()
			}
		} else {
			k.logger.Error("lazy-load failed", "id", msg.Target, "error", err)
			// Return error response if this is a request
			if msg.Type == api.TypeRequest || msg.Type == "" {
				resp := api.Message{
					ID:        msg.ID + "-resp",
					Type:      api.TypeResponse,
					Sender:    "system",
					Target:    msg.Sender,
					Payload:   []byte(fmt.Sprintf(`{"error":"failed to load plugin: %s"}`, err.Error())),
					Timestamp: time.Now().Unix(),
				}
				k.RouteMessage(ctx, resp)
			}
		}
		return
	}

	k.logger.Warn("message target not found", "target", msg.Target)

	// Return error response if this is a request
	if msg.Type == api.TypeRequest || msg.Type == "" {
		resp := api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    "system",
			Target:    msg.Sender,
			Payload:   []byte(fmt.Sprintf(`{"error":"target service or capability not found: %s"}`, msg.Target)),
			Timestamp: time.Now().Unix(),
		}
		k.RouteMessage(ctx, resp)
	}
}

func (k *WITKernel) HandleMessageSync(ctx context.Context, msg api.Message) (api.Message, error) {
	return k.handleMessageSync(ctx, msg)
}

// handleMessageSync handles a synchronous message call.
func (k *WITKernel) handleMessageSync(ctx context.Context, msg api.Message) (api.Message, error) {
	k.mu.RLock()
	// Attempt capability resolution first
	if capTarget, ok := k.capabilityMap[msg.Target]; ok {
		if _, exists := k.plugins[msg.Target]; !exists {
			msg.Target = capTarget
		}
	}

	plugin, ok := k.plugins[msg.Target]
	_, hasMeta := k.metadata[msg.Target]
	k.mu.RUnlock()

	if !ok && hasMeta {
		// Check for lazy load - release RLock first to avoid deadlock during Lock()
		if err := k.lazyLoadPlugin(ctx, msg.Target); err == nil {
			k.mu.RLock()
			plugin, ok = k.plugins[msg.Target]
			k.mu.RUnlock()
		}
	}

	if ok && plugin != nil {
		return plugin.HandleMessage(ctx, msg)
	}

	return api.Message{}, fmt.Errorf("plugin %s not found and lazy-load failed or no metadata", msg.Target)
}

// lazyLoadPlugin loads a plugin that was registered but not yet instantiated.
func (k *WITKernel) lazyLoadPlugin(ctx context.Context, pluginID string) error {
	k.mu.Lock()
	if _, ok := k.plugins[pluginID]; ok {
		k.mu.Unlock()
		return nil
	}
	if _, ok := k.loading[pluginID]; ok {
		ch := k.loading[pluginID]
		k.mu.Unlock()
		select {
		case <-ch:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ch := make(chan struct{})
	k.loading[pluginID] = ch
	k.mu.Unlock()

	loader, ok := k.loaders[pluginID]
	if !ok {
		k.cleanupLoading(pluginID, ch)
		return errors.New("loader not found")
	}

	plugin, err := loader.LoadPlugin(ctx, pluginID)
	if err != nil {
		k.cleanupLoading(pluginID, ch)
		return err
	}

	k.mu.Lock()
	k.plugins[pluginID] = plugin
	delete(k.loading, pluginID)
	close(ch)
	// Track plugin count for WITKernel if it had telemetry (it doesn't yet)
	k.mu.Unlock()

	return nil
}

func (k *WITKernel) cleanupLoading(id string, ch chan struct{}) {
	k.mu.Lock()
	delete(k.loading, id)
	close(ch)
	k.mu.Unlock()
}

// RegisterPluginLoader registers a loader for a plugin.
func (k *WITKernel) RegisterPluginLoader(pluginID string, loader api.PluginLoader, metadata api.PluginMetadata) {
	k.mu.Lock()
	k.loaders[pluginID] = loader
	k.metadata[pluginID] = metadata
	
	for _, cap := range metadata.Capabilities {
		if cap.Method != "" {
			k.capabilityMap[cap.Method] = pluginID
		}
	}
	k.mu.Unlock()

	// Broadcast registration for lazy loader
	reg := struct {
		ID           string           `json:"id"`
		Type         string           `json:"type"`
		Capabilities []api.Capability `json:"capabilities"`
	}{
		ID:           pluginID,
		Type:         "plugin",
		Capabilities: metadata.Capabilities,
	}
	payload, _ := json.Marshal(reg)
	go k.RouteMessage(context.Background(), api.Message{
		ID:        "reg-evt-lazy-" + pluginID,
		Type:      api.TypeEvent,
		Sender:    "kernel",
		Target:    "events",
		Method:    "publish",
		Payload:   []byte(fmt.Sprintf(`{"topic":"component:registered","data":%s}`, string(payload))),
		Timestamp: time.Now().Unix(),
	})
}

// RegisterPlugin registers an already instantiated plugin.
func (k *WITKernel) RegisterPlugin(plugin api.Plugin) {
	id := plugin.ID()

	k.mu.Lock()
	if existing, ok := k.plugins[id]; ok && existing != nil {
		// For internal core plugins, we avoid re-registering to preserve state/subscriptions
		if id == "events" || id == "command-manager" || id == "wasm-manager" {
			k.mu.Unlock()
			k.logger.Debug("skipping re-registration of internal core plugin", "id", id)
			return
		}
	} else {
		k.telemetry.PluginCountChange(context.Background(), 1)
	}

	k.plugins[id] = plugin
	k.metadata[id] = api.PluginMetadata{
		ID:           id,
		Capabilities: plugin.Capabilities(),
	}

	// Update capability map
	for _, cap := range plugin.Capabilities() {
		if cap.Method != "" {
			k.capabilityMap[cap.Method] = id
		}
	}
	k.mu.Unlock()

	// If the plugin needs a router, provide it
	type routerAcceptor interface {
		SetRouter(func(context.Context, api.Message))
	}
	if ra, ok := plugin.(routerAcceptor); ok {
		ra.SetRouter(func(ctx context.Context, msg api.Message) {
			k.RouteMessage(ctx, msg)
		})
	}

	// Broadcast registration event
	reg := struct {
		ID           string           `json:"id"`
		Type         string           `json:"type"`
		Capabilities []api.Capability `json:"capabilities"`
	}{
		ID:           id,
		Type:         "plugin",
		Capabilities: plugin.Capabilities(),
	}
	payload, _ := json.Marshal(reg)
	go k.RouteMessage(context.Background(), api.Message{
		ID:        "reg-evt-" + id,
		Type:      api.TypeEvent,
		Sender:    "kernel",
		Target:    "events",
		Method:    "publish",
		Payload:   []byte(fmt.Sprintf(`{"topic":"component:registered","data":%s}`, string(payload))),
		Timestamp: time.Now().Unix(),
	})
}

// RegisterWASMPluginAtScale registers a WASM plugin with explicit resource limits.
func (k *WITKernel) RegisterWASMPluginAtScale(pluginID string, wasmBytes []byte, maxMemoryMB uint32, msgPerSec int, caps []api.Capability) error {
	plugin := &witPluginWrapper{
		id:      pluginID,
		manager: k.wasmManager,
		caps:    caps,
	}

	k.RegisterPlugin(plugin)

	if err := k.wasmManager.LoadPlugin(context.Background(), pluginID, wasmBytes, maxMemoryMB, msgPerSec, caps); err != nil {
		return err
	}

	// Explicitly set metadata to boot load time
	k.mu.Lock()
	meta := k.metadata[pluginID]
	meta.LoadTime = api.LoadTimeBoot
	k.metadata[pluginID] = meta
	k.mu.Unlock()

	return nil
}

// RegisterWASMPlugin registers a WASM plugin with the kernel using default limits.
func (k *WITKernel) RegisterWASMPlugin(pluginID string, wasmBytes []byte, caps []api.Capability) error {
	return k.RegisterWASMPluginAtScale(pluginID, wasmBytes, 128, 1000, caps)
}

// RegisterFrontend registers a frontend connection.
func (k *WITKernel) RegisterFrontend(connID string, ch chan<- api.Message) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.frontends[connID] = ch
}

// UnregisterFrontend unregisters a frontend connection.
func (k *WITKernel) UnregisterFrontend(connID string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.frontends, connID)
}

// RegisterInterceptor registers a message interceptor.
func (k *WITKernel) RegisterInterceptor(interceptor api.Interceptor) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.interceptors = append(k.interceptors, interceptor)
}

// GetPluginMetadata gets metadata for all registered plugins.
func (k *WITKernel) GetPluginMetadata() map[string]api.PluginMetadata {
	k.mu.RLock()
	defer k.mu.RUnlock()
	metadata := make(map[string]api.PluginMetadata, len(k.metadata))
	for id, md := range k.metadata {
		metadata[id] = md
	}
	return metadata
}

// GetPlugin gets a plugin instance by ID.
func (k *WITKernel) GetPlugin(pluginID string) (api.Plugin, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	plugin, ok := k.plugins[pluginID]
	return plugin, ok
}

// Workspace Management

func (k *WITKernel) RegisterWorkspace(ws api.Workspace) {
	k.wasmManager.RegisterWorkspace(ws)
}

func (k *WITKernel) UnregisterWorkspace(id string) {
	k.wasmManager.UnregisterWorkspace(id)
}

func (k *WITKernel) SetActiveWorkspace(id string) {
	k.wasmManager.SetActiveWorkspace(id)
}

func (k *WITKernel) GetActiveWorkspace() (api.Workspace, bool) {
	return k.wasmManager.GetActiveWorkspace()
}

func (k *WITKernel) ListWorkspaces() []api.Workspace {
	return k.wasmManager.ListWorkspaces()
}

func (k *WITKernel) ListWidgets() []api.Widget {
	return k.wasmManager.ListWidgets()
}

// Shutdown gracefully shuts down the kernel.
func (k *WITKernel) Shutdown(ctx context.Context) error {
	k.logger.Info("shutting down kernel")
	k.stopOnce.Do(func() {
		close(k.stopCh)
	})
	k.mu.Lock()
	plugins := make([]api.Plugin, 0, len(k.plugins))
	for _, plugin := range k.plugins {
		plugins = append(plugins, plugin)
	}
	k.mu.Unlock()

	for _, plugin := range plugins {
		_ = plugin.Shutdown(ctx)
	}

	return k.wasmManager.Close(ctx)
}

// witPluginWrapper wraps a WASM plugin for the kernel API.
type witPluginWrapper struct {
	id      string
	manager *wasm.Manager
	caps    []api.Capability
}

func (p *witPluginWrapper) ID() string                     { return p.id }
func (p *witPluginWrapper) Capabilities() []api.Capability { return p.caps }

func (p *witPluginWrapper) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	if msg.Type == api.TypeRequest {
		err := p.manager.RouteMessage(ctx, p.id, msg)
		if err != nil {
			return api.Message{}, err
		}
		resp, err := p.manager.GetResponse(ctx, p.id, msg.ID)
		return resp, err
	}
	// For events, just route and return empty
	err := p.manager.RouteMessage(ctx, p.id, msg)
	return api.Message{}, err
}

func (p *witPluginWrapper) Shutdown(ctx context.Context) error {
	return p.manager.UnloadPlugin(ctx, p.id)
}

// wasmManagerPlugin wraps the WASM manager as a native kernel plugin.
type wasmManagerPlugin struct {
	kernel *WITKernel
}

func (w *wasmManagerPlugin) ID() string { return "wasm-manager" }
func (w *wasmManagerPlugin) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "load", Description: "Load a WASM plugin"},
		{Method: "unload", Description: "Unload a WASM plugin"},
		{Method: "watch", Description: "Watch a WASM plugin for hot-reload"},
	}
}

func (w *wasmManagerPlugin) Shutdown(ctx context.Context) error { return nil }

func (w *wasmManagerPlugin) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "load":
		var req struct {
			ID           string           `json:"id"`
			Path         string           `json:"path"`
			MaxMemoryMB  uint32           `json:"max_memory_mb"`
			MsgPerSecond int              `json:"msg_per_second"`
			Capabilities []api.Capability `json:"capabilities"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		wasmBytes, err := os.ReadFile(req.Path)
		if err != nil {
			return api.Message{}, err
		}

		// Defaults if not provided
		if req.MaxMemoryMB == 0 {
			req.MaxMemoryMB = 128
		}
		if req.MsgPerSecond == 0 {
			req.MsgPerSecond = 1000
		}

		if err := w.kernel.RegisterWASMPluginAtScale(req.ID, wasmBytes, req.MaxMemoryMB, req.MsgPerSecond, req.Capabilities); err != nil {
			return api.Message{}, err
		}

		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  w.ID(),
			Target:  msg.Sender,
			Payload: []byte(`{"status":"loaded"}`),
		}, nil

	case "unload":
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}
		if err := w.kernel.wasmManager.UnloadPlugin(ctx, req.ID); err != nil {
			return api.Message{}, err
		}
		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  w.ID(),
			Target:  msg.Sender,
			Payload: []byte(`{"status":"unloaded"}`),
		}, nil

	case "watch":
		// This is a placeholder for actual hot-reload logic
		// In a real implementation, we would start a file watcher
		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  w.ID(),
			Target:  msg.Sender,
			Payload: []byte(`{"status":"watching"}`),
		}, nil
	}
	return api.Message{}, fmt.Errorf("unknown method: %s", msg.Method)
}

func (k *WITKernel) handleInternalMessage(ctx context.Context, msg api.Message) {
	if msg.Type != api.TypeRequest {
		return
	}

	k.logger.Debug("handling internal message", "method", msg.Method)

	switch msg.Method {
	case "ping":
		resp := api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    "kernel",
			Target:    msg.Sender,
			Method:    "ping",
			Payload:   []byte(`{"status":"pong"}`),
			Timestamp: time.Now().Unix(),
		}
		k.RouteMessage(ctx, resp)
	case "dashboard:list-widgets":
		widgets := k.ListWidgets()
		payload, _ := json.Marshal(widgets)
		resp := api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    "kernel",
			Target:    msg.Sender,
			Method:    "dashboard:list-widgets",
			Payload:   payload,
			Timestamp: time.Now().Unix(),
		}
		k.RouteMessage(ctx, resp)
	case "stop":
		k.logger.Info("stop request received via internal channel")
		k.stopOnce.Do(func() {
			close(k.stopCh)
		})
	default:
		k.logger.Warn("unknown internal method", "method", msg.Method)
	}
}
