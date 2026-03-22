package kernel

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
	"github.com/jnesbitt/alloy-go/pkg/wasm"
)

// WITKernel is a kernel implementation that uses the WIT-based WASM runtime.
type WITKernel struct {
	logger      *slog.Logger
	mu          sync.RWMutex
	plugins     map[string]api.Plugin
	metadata    map[string]api.PluginMetadata
	loaders     map[string]api.PluginLoader
	frontends   map[string]chan<- api.Message
	interceptors []api.Interceptor
	loading     map[string]chan struct{}
	stopCh      chan struct{}
	wasmManager *wasm.Manager
	storage     storage.StateStore
	dataDir     string
}

// NewWITKernel creates a new WIT-based kernel.
func NewWITKernel(
	logger *slog.Logger,
	storage storage.StateStore,
	dataDir string,
) (*WITKernel, error) {
	// Create the kernel
	kernel := &WITKernel{
		logger:      logger,
		plugins:     make(map[string]api.Plugin),
		metadata:    make(map[string]api.PluginMetadata),
		loaders:     make(map[string]api.PluginLoader),
		frontends:   make(map[string]chan<- api.Message),
		loading:     make(map[string]chan struct{}),
		stopCh:      make(chan struct{}),
		storage:     storage,
		dataDir:     dataDir,
	}

	// Create the WASM manager
	router := func(ctx context.Context, msg api.Message) {
		kernel.routeMessage(ctx, msg)
	}

	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		return kernel.handleMessageSync(ctx, msg)
	}

	wasmManager, err := wasm.NewManager(logger, storage, dataDir, router, call)
	if err != nil {
		return nil, err
	}

	kernel.wasmManager = wasmManager

	// Start the monitor
	kernel.wasmManager.StartMonitor(context.Background(), 30*time.Second)

	return kernel, nil
}

// routeMessage routes messages to the appropriate destination.
func (k *WITKernel) routeMessage(ctx context.Context, msg api.Message) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	// Apply interceptors
	for _, interceptor := range k.interceptors {
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
		for connID, ch := range k.frontends {
			select {
			case ch <- msg:
			case <-ctx.Done():
			case <-k.stopCh:
			default:
				k.logger.Warn("frontend channel full", "connID", connID)
			}
		}
		return
	}

	// Route to a specific plugin or frontend
	if ch, ok := k.frontends[msg.Target]; ok {
		select {
		case ch <- msg:
		default:
			k.logger.Warn("frontend channel full", "target", msg.Target)
		}
	} else if plugin, ok := k.plugins[msg.Target]; ok {
		// Asynchronous routing: we use a background task to handle it via HandleMessage
		go func() {
			_, _ = plugin.HandleMessage(ctx, msg)
		}()
	} else if _, ok := k.metadata[msg.Target]; ok {
		// Target is a lazy-loaded plugin
		if err := k.lazyLoadPlugin(ctx, msg.Target); err == nil {
			if plugin, ok := k.plugins[msg.Target]; ok {
				go func() { _, _ = plugin.HandleMessage(ctx, msg) }()
			}
		}
	} else {
		k.logger.Warn("message target not found", "target", msg.Target)
	}
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
		case <-ch: return nil
		case <-ctx.Done(): return ctx.Err()
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
func (k *WITKernel) RegisterPluginLoader(pluginID string, loader api.PluginLoader) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.loaders[pluginID] = loader
	// Note: api.PluginLoader doesn't have Metadata() method in current api.
	// We might need to assume metadata is provided elsewhere or handle it.
}

// RegisterPlugin registers an already instantiated plugin.
func (k *WITKernel) RegisterPlugin(plugin api.Plugin) {
	k.mu.Lock()
	defer k.mu.Unlock()
	id := plugin.ID()
	k.plugins[id] = plugin
	k.metadata[id] = api.PluginMetadata{
		ID: id,
		Capabilities: plugin.Capabilities(),
	}
}

// RegisterWASMPlugin registers a WASM plugin with the kernel.
func (k *WITKernel) RegisterWASMPlugin(pluginID string, wasmBytes []byte, caps []api.Capability) error {
	if err := k.wasmManager.LoadPlugin(context.Background(), pluginID, wasmBytes, caps); err != nil {
		return err
	}

	plugin := &witPluginWrapper{
		id:       pluginID,
		manager:  k.wasmManager,
		caps:     caps,
	}

	k.RegisterPlugin(plugin)
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
	id       string
	manager  *wasm.Manager
	caps     []api.Capability
}

func (p *witPluginWrapper) ID() string { return p.id }
func (p *witPluginWrapper) Capabilities() []api.Capability { return p.caps }

func (p *witPluginWrapper) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	if msg.Type == api.TypeRequest {
		err := p.manager.RouteMessage(ctx, p.id, msg)
		if err != nil { return api.Message{}, err }
		return p.manager.GetResponse(ctx, p.id)
	}
	// For events, just route and return empty
	err := p.manager.RouteMessage(ctx, p.id, msg)
	return api.Message{}, err
}

func (p *witPluginWrapper) Shutdown(ctx context.Context) error {
	return p.manager.UnloadPlugin(ctx, p.id)
}
