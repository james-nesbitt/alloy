package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

	// Internal core services
	events   api.Plugin
	commands api.Plugin
}

// NewWITKernel creates a new WIT-based kernel.
func NewWITKernel(
	logger *slog.Logger,
	storage storage.StateStore,
	dataDir string,
) (*WITKernel, error) {
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

	kernel.commands = native.NewCommandManager(logger)
	kernel.RegisterPlugin(kernel.commands)

	// Start the monitor
	kernel.wasmManager.StartMonitor(context.Background(), 30*time.Second)

	return kernel, nil
}

// routeMessage routes messages to the appropriate destination.
func (k *WITKernel) RouteMessage(ctx context.Context, msg api.Message) {
	k.logger.Debug("kernel routing message", "id", msg.ID, "sender", msg.Sender, "target", msg.Target, "method", msg.Method)

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

	k.logger.Debug("routing to specific target", "target", msg.Target, "msgID", msg.ID)

	k.mu.RLock()
	ch, isFrontend := k.frontends[msg.Target]
	plugin, isPlugin := k.plugins[msg.Target]
	_, isLazy := k.metadata[msg.Target]
	k.mu.RUnlock()

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
}

// handleMessageSync handles a synchronous message call.
func (k *WITKernel) handleMessageSync(ctx context.Context, msg api.Message) (api.Message, error) {
	k.mu.RLock()
	plugin, ok := k.plugins[msg.Target]
	k.mu.RUnlock()

	if !ok {
		// Check for lazy load
		if _, exists := k.metadata[msg.Target]; exists {
			if err := k.lazyLoadPlugin(ctx, msg.Target); err == nil {
				k.mu.RLock()
				plugin, ok = k.plugins[msg.Target]
				k.mu.RUnlock()
			}
		}
	}

	if ok {
		return plugin.HandleMessage(ctx, msg)
	}

	return api.Message{}, errors.New("plugin not found")
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
	k.plugins[id] = plugin
	k.metadata[id] = api.PluginMetadata{
		ID:           id,
		Capabilities: plugin.Capabilities(),
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

// RegisterWASMPlugin registers a WASM plugin with the kernel.
func (k *WITKernel) RegisterWASMPlugin(pluginID string, wasmBytes []byte, caps []api.Capability) error {
	plugin := &witPluginWrapper{
		id:      pluginID,
		manager: k.wasmManager,
		caps:    caps,
	}

	k.RegisterPlugin(plugin)

	if err := k.wasmManager.LoadPlugin(context.Background(), pluginID, wasmBytes, caps); err != nil {
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
	case "stop":
		k.logger.Info("stop request received via internal channel")
		k.stopOnce.Do(func() {
			close(k.stopCh)
		})
	default:
		k.logger.Warn("unknown internal method", "method", msg.Method)
	}
}
