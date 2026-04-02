package wasm

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
	"github.com/james-nesbitt/alloy/pkg/wasm/runtime"
)

// Manager handles WASM plugin instances.
type Manager struct {
	runtime  *runtime.Runtime
	logger   *slog.Logger
	plugins  map[string]*PluginInstance
	mu       sync.RWMutex
	routerFn func(ctx context.Context, msg api.Message)
	callFn   func(ctx context.Context, msg api.Message) (api.Message, error)

	// Buffers (Integrated)
	buffers api.MmapRegistry
}

// PluginInstance represents a loaded plugin instance.
type PluginInstance struct {
	ID           string
	Instance     *runtime.Instance
	Capabilities []api.Capability
	Status       runtime.Status
	Metadata     api.PluginMetadata
}

// NewManager creates a new WASM manager.
func NewManager(
	logger *slog.Logger,
	kv storage.StateStore,
	dataDir string,
	bufferRegistry api.MmapRegistry,
	router func(ctx context.Context, msg api.Message),
	call func(ctx context.Context, msg api.Message) (api.Message, error),
) (*Manager, error) {
	// Create the runtime
	rt, err := runtime.NewRuntime(context.Background(), logger, kv, dataDir, bufferRegistry, router, call)
	if err != nil {
		return nil, err
	}

	return &Manager{
		runtime:  rt,
		logger:   logger,
		plugins:  make(map[string]*PluginInstance),
		routerFn: router,
		callFn:   call,
		buffers:  bufferRegistry,
	}, nil
}

// LoadPlugin loads a WASM plugin with resource limits. (Phase 10)
func (m *Manager) LoadPlugin(
	ctx context.Context,
	id string,
	wasmBytes []byte,
	maxMemoryMB uint32,
	msgPerSec int,
	caps []api.Capability,
	background bool,
) error {
	// Load the plugin in the runtime
	instance, err := m.runtime.LoadPlugin(ctx, id, wasmBytes, maxMemoryMB, msgPerSec, caps, background)
	if err != nil {
		return err
	}

	// Create the plugin instance (Phase 10)
	plugin := &PluginInstance{
		ID:           id,
		Instance:     instance,
		Capabilities: caps,
		Status:       runtime.StatusRunning,
		Metadata: api.PluginMetadata{
			ID:           id,
			Capabilities: caps,
			Background:   background,
		},
	}

	// Register the plugin
	m.mu.Lock()
	m.plugins[id] = plugin
	m.mu.Unlock()

	// Update metadata when it becomes available
	go func() {
		// Wait for plugin to initialize
		time.Sleep(200 * time.Millisecond)

		// Get updated metadata from the instance
		m.mu.Lock()
		if inst, ok := m.plugins[id]; ok {
			inst.Metadata = instance.Metadata()
		}
		m.mu.Unlock()
	}()

	return nil
}

// UnloadPlugin unloads a WASM plugin.
func (m *Manager) UnloadPlugin(ctx context.Context, id string) error {
	m.mu.Lock()
	plugin, ok := m.plugins[id]
	if ok {
		delete(m.plugins, id)
	}
	m.mu.Unlock()

	if ok {
		return plugin.Instance.Close(ctx)
	}

	return nil
}

// RouteMessage routes a message to a plugin.
func (m *Manager) RouteMessage(ctx context.Context, pluginID string, msg api.Message) error {
	return m.runtime.RouteMessage(ctx, pluginID, msg)
}

// SetCall overrides the internal call function.
func (m *Manager) SetCall(call func(ctx context.Context, msg api.Message) (api.Message, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callFn = call
}


// Call is a helper that sends a message and waits for a response synchronously.
func (m *Manager) Call(ctx context.Context, pluginID string, msg api.Message) (api.Message, error) {
	if err := m.RouteMessage(ctx, pluginID, msg); err != nil {
		return api.Message{}, err
	}
	return m.GetResponse(ctx, pluginID, msg.ID)
}


// GetResponse gets a response from a plugin.
func (m *Manager) GetResponse(ctx context.Context, pluginID string, requestID string) (api.Message, error) {
	return m.runtime.GetResponse(ctx, pluginID, requestID)
}

// GetPluginCapabilities gets the capabilities of a plugin.
func (m *Manager) GetPluginCapabilities(pluginID string) ([]api.Capability, bool) {
	m.mu.RLock()
	plugin, ok := m.plugins[pluginID]
	m.mu.RUnlock()

	if ok {
		return plugin.Capabilities, true
	}

	return nil, false
}

// GetPluginStatus gets the status of a plugin.
// GetPluginMetadata gets the metadata of a plugin.
func (m *Manager) GetPluginMetadata(pluginID string) (api.PluginMetadata, bool) {
	m.mu.RLock()
	plugin, ok := m.plugins[pluginID]
	m.mu.RUnlock()

	if ok {
		return plugin.Metadata, true
	}

	return api.PluginMetadata{}, false
}

// GetPluginStatus gets the status of a plugin.
func (m *Manager) GetPluginStatus(pluginID string) (runtime.Status, bool) {
	m.mu.RLock()
	plugin, ok := m.plugins[pluginID]
	m.mu.RUnlock()

	if ok {
		return plugin.Status, true
	}

	return runtime.StatusStopped, false
}

// DiscoverPlugins finds plugins that provide a specific capability.
// DiscoveryPlugins finds plugins that provide a specific capability (or satisfy an intent).
func (m *Manager) DiscoverPlugins(capabilityMethod string) []api.PluginMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []api.PluginMetadata

	for _, plugin := range m.plugins {
		matched := false
		for _, cap := range plugin.Metadata.Capabilities {
			if cap.Method == capabilityMethod {
				matched = true
				break
			}
			for _, intent := range cap.Intents {
				if intent == capabilityMethod {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			for _, intent := range plugin.Metadata.Intents {
				if intent == capabilityMethod {
					matched = true
					break
				}
			}
		}
		if matched {
			results = append(results, plugin.Metadata)
		}
	}

	return results
}

// DiscoverPluginsByTag finds plugins with a specific tag (currently metadata is minimal)
func (m *Manager) DiscoverPluginsByTag(tag string) []api.PluginMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []api.PluginMetadata
	// Current api.PluginMetadata doesn't have tags in root, but it might later.
	return results
}

// GetAllPluginMetadata gets metadata for all loaded plugins.
func (m *Manager) GetAllPluginMetadata() []api.PluginMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metadata := make([]api.PluginMetadata, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		metadata = append(metadata, plugin.Metadata)
	}

	return metadata
}

// Close shuts down the manager.
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close all plugins
	for id := range m.plugins {
		if err := m.plugins[id].Instance.Close(ctx); err != nil {
			m.logger.Error("failed to close plugin", "id", id, "error", err)
		}
	}

	// Close the runtime
	return m.runtime.Close(ctx)
}

// StartMonitor starts a background monitor for plugin health.
func (m *Manager) StartMonitor(ctx context.Context, interval time.Duration) {
	// Use a dedicated goroutine that respects BOTH manager closure and system context
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				m.logger.Debug("stopping WASM monitor (manager context done)")
				return
			case <-ticker.C:
				m.checkPluginsHealth()
			}
		}
	}()
}

// checkPluginsHealth checks the health of all plugins.
func (m *Manager) checkPluginsHealth() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, plugin := range m.plugins {
		// In a real implementation, we would check plugin health
		// For now, just log the status
		m.logger.Debug("plugin status", "id", id, "status", plugin.Status)
	}
}
