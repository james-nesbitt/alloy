package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
	"github.com/jnesbitt/alloy-go/pkg/wasm2"
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
	wasmManager *wasm2.Manager
	storage     storage.StateStore
	dataDir     string
}

// NewWITKernel creates a new WIT-based kernel.
func NewWITKernel(
	logger *slog.Logger,
	storage storage.StateStore,
	dataDir string,
) (*WITKernel, error) {
	// Create the WASM manager
	router := func(ctx context.Context, msg api.Message) {
		// This will be set up properly below
	}

	call := func(ctx context.Context, msg api.Message) (api.Message, error) {
		return api.Message{}, errors.New("not implemented")
	}

	wasmManager, err := wasm2.NewManager(logger, storage, dataDir, router, call)
	if err != nil {
		return nil, err
	}

	// Create the kernel
	kernel := &WITKernel{
		logger:      logger,
		plugins:     make(map[string]api.Plugin),
		metadata:    make(map[string]api.PluginMetadata),
		loaders:     make(map[string]api.PluginLoader),
		frontends:   make(map[string]chan<- api.Message),
		loading:     make(map[string]chan struct{}),
		stopCh:      make(chan struct{}),
		wasmManager: wasmManager,
		storage:     storage,
		dataDir:     dataDir,
	}

	// Set up the router and call functions
	router = kernel.routeMessage
	call = kernel.callPlugin

	// Update the WASM manager with the proper functions
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
		var err error
		msg, err = interceptor.Intercept(ctx, msg)
		if err != nil {
			k.logger.Error("interceptor failed", "error", err)
			return
		}
		if msg.Type == "dropped" {
			return
		}
	}

	// Route to frontends if this is a broadcast
	if msg.Target == "" || msg.Target == "*" {
		for connID, ch := range k.frontends {
			select {
			case ch <- msg:
			case <-ctx.Done():
				k.logger.Warn("context done while sending to frontend", "connID", connID)
			case <-k.stopCh:
				k.logger.Warn("kernel stopping while sending to frontend", "connID", connID)
			default:
				k.logger.Warn("frontend channel full", "connID", connID)
			}
		}
		return
	}

	// Route to a specific plugin or frontend
	if ch, ok := k.frontends[msg.Target]; ok {
		// Target is a frontend
		select {
		case ch <- msg:
		case <-ctx.Done():
			k.logger.Warn("context done while sending to frontend", "target", msg.Target)
		case <-k.stopCh:
			k.logger.Warn("kernel stopping while sending to frontend", "target", msg.Target)
		default:
			k.logger.Warn("frontend channel full", "target", msg.Target)
		}
	} else if plugin, ok := k.plugins[msg.Target]; ok {
		// Target is a plugin
		if err := plugin.RouteMessage(ctx, msg); err != nil {
			k.logger.Error("failed to route message to plugin", "target", msg.Target, "error", err)
		}
	} else if _, ok := k.metadata[msg.Target]; ok {
		// Target is a lazy-loaded plugin
		if err := k.lazyLoadPlugin(ctx, msg.Target); err != nil {
			k.logger.Error("failed to lazy load plugin", "target", msg.Target, "error", err)
		} else if plugin, ok := k.plugins[msg.Target]; ok {
			if err := plugin.RouteMessage(ctx, msg); err != nil {
				k.logger.Error("failed to route message to lazy-loaded plugin", "target", msg.Target, "error", err)
			}
		}
	} else {
		k.logger.Warn("message target not found", "target", msg.Target)
	}
}

// callPlugin makes a synchronous call to a plugin.
func (k *WITKernel) callPlugin(ctx context.Context, msg api.Message) (api.Message, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	// Check if the target is a plugin
	if plugin, ok := k.plugins[msg.Target]; ok {
		return plugin.Call(ctx, msg)
	}

	// Check if the target is a lazy-loaded plugin
	if _, ok := k.metadata[msg.Target]; ok {
		if err := k.lazyLoadPlugin(ctx, msg.Target); err != nil {
			return api.Message{}, err
		}
		if plugin, ok := k.plugins[msg.Target]; ok {
			return plugin.Call(ctx, msg)
		}
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

	// Load the plugin
	loader, ok := k.loaders[pluginID]
	if !ok {
		k.mu.Lock()
		delete(k.loading, pluginID)
		close(ch)
		k.mu.Unlock()
		return errors.New("loader not found")
	}

	plugin, err := loader.Load(ctx)
	if err != nil {
		k.mu.Lock()
		delete(k.loading, pluginID)
		close(ch)
		k.mu.Unlock()
		return err
	}

	// Register the plugin
	k.mu.Lock()
	k.plugins[pluginID] = plugin
	delete(k.loading, pluginID)
	close(ch)
	k.mu.Unlock()

	return nil
}

// RegisterPluginLoader registers a loader for a plugin.
func (k *WITKernel) RegisterPluginLoader(pluginID string, loader api.PluginLoader) {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.loaders[pluginID] = loader
	k.metadata[pluginID] = loader.Metadata()
}

// RegisterPlugin registers an already instantiated plugin.
func (k *WITKernel) RegisterPlugin(plugin api.Plugin) {
	k.mu.Lock()
	defer k.mu.Unlock()

	pluginID := plugin.Metadata().ID
	k.plugins[pluginID] = plugin
	k.metadata[pluginID] = plugin.Metadata()
}

// RegisterWASMPlugin registers a WASM plugin with the kernel.
func (k *WITKernel) RegisterWASMPlugin(pluginID string, wasmBytes []byte, caps []api.Capability) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	// Load the plugin in the WASM manager
	if err := k.wasmManager.LoadPlugin(context.Background(), pluginID, wasmBytes, caps); err != nil {
		return err
	}

	// Create a plugin wrapper
	plugin := &witPluginWrapper{
		id:       pluginID,
		manager:  k.wasmManager,
		metadata: api.PluginMetadata{
			ID:          pluginID,
			Type:        "wasm",
			Capabilities: caps,
		},
	}

	// Register the plugin
	k.plugins[pluginID] = plugin
	k.metadata[pluginID] = plugin.metadata

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

	// Create a copy of the metadata
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

	// Signal shutdown
	close(k.stopCh)

	// Close all plugins
	k.mu.Lock()
	plugins := make([]api.Plugin, 0, len(k.plugins))
	for _, plugin := range k.plugins {
		plugins = append(plugins, plugin)
	}
	k.mu.Unlock()

	// Close plugins with timeout
	errCh := make(chan error, len(plugins))
	for _, plugin := range plugins {
		go func(p api.Plugin) {
			errCh <- p.Close(ctx)
		}(plugin)
	}

	// Wait for plugins to close or timeout
	select {
	case err := <-errCh:
		if err != nil {
			k.logger.Error("plugin shutdown error", "error", err)
		}
	case <-ctx.Done():
		k.logger.Warn("shutdown timed out")
	}

	// Close the WASM manager
	return k.wasmManager.Close(ctx)
}

// witPluginWrapper wraps a WASM plugin for the kernel API.
type witPluginWrapper struct {
	id       string
	manager  *wasm2.Manager
	metadata api.PluginMetadata
}

// Metadata returns the plugin metadata.
func (p *witPluginWrapper) Metadata() api.PluginMetadata {
	return p.metadata
}

// RouteMessage routes a message to the plugin.
func (p *witPluginWrapper) RouteMessage(ctx context.Context, msg api.Message) error {
	return p.manager.RouteMessage(ctx, p.id, msg)
}

// Call makes a synchronous call to the plugin.
func (p *witPluginWrapper) Call(ctx context.Context, msg api.Message) (api.Message, error) {
	return p.manager.GetResponse(ctx, p.id)
}

// Close shuts down the plugin.
func (p *witPluginWrapper) Close(ctx context.Context) error {
	return p.manager.UnloadPlugin(ctx, p.id)
}