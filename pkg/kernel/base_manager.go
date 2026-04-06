package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

// BaseInstance represents an active Base in the system.
type BaseInstance struct {
	ID        string
	Manifest  *api.BaseManifest
	Metadata  map[string]any
	Plugins   map[string]api.Plugin // PluginID -> Instance (for Multi-instance)
	CreatedAt time.Time
}

// BaseManager handles the lifecycle and isolation of multiple active Bases.
type BaseManager struct {
	logger *slog.Logger
	mu     sync.RWMutex
	bases  map[string]*BaseInstance
	kernel *Kernel
}

func NewBaseManager(logger *slog.Logger, kernel *Kernel) *BaseManager {
	return &BaseManager{
		logger: logger,
		bases:  make(map[string]*BaseInstance),
		kernel: kernel,
	}
}

func (b *BaseManager) ID() string { return "base-manager" }

func (b *BaseManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "activate", Description: "Activate a Base using a specific discovery capability"},
		{Method: "list", Description: "List all active Bases"},
	}
}

func (b *BaseManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "activate":
		var req struct {
			BaseID     string         `json:"base_id"`
			Capability string         `json:"capability"`
			Metadata   map[string]any `json:"metadata,omitempty"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return b.errorResponse(msg, err), nil
		}
		if err := b.Activate(ctx, req.BaseID, req.Capability, req.Metadata); err != nil {
			return b.errorResponse(msg, err), nil
		}
		return b.successResponse(msg), nil

	case "list":
		b.mu.RLock()
		defer b.mu.RUnlock()
		var list []map[string]any
		for _, base := range b.bases {
			list = append(list, map[string]any{
				"id":         base.ID,
				"created_at": base.CreatedAt,
			})
		}
		payload, _ := json.Marshal(list)
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    b.ID(),
			Target:    msg.Sender,
			Payload:   payload,
			Timestamp: time.Now().Unix(),
		}, nil
	}
	return api.Message{}, nil
}

func (b *BaseManager) Activate(ctx context.Context, baseID, capability string, metadata map[string]any) error {
	// Phase 13: Capability-Led Discovery
	b.mu.RLock()
	kernel := b.kernel
	b.mu.RUnlock()

	// 1. Find a plugin that advertises this discovery capability
	var providerID string
	allMeta := kernel.GetPluginMetadata()
	for id, meta := range allMeta {
		for _, cap := range meta.Capabilities {
			if cap.Advertised && cap.Method == capability {
				providerID = id
				break
			}
		}
		if providerID != "" {
			break
		}
	}

	if providerID == "" {
		return fmt.Errorf("no plugin found advertising capability: %s", capability)
	}

	b.logger.Info("activating base via capability delegation", "id", baseID, "capability", capability, "provider", providerID)

	// 2. Ensure provider is loaded with necessary mounts and send discovery request
	mounts := make(map[string]string)
	if path, ok := metadata["project_path"].(string); ok {
		mounts[path] = "/work"
	}

	// Trigger lazy-load with mounts if not already loaded
	if _, ok := kernel.GetPlugin(providerID); !ok {
		// Use Metadata to find the loader
		pluginMetadata := kernel.GetPluginMetadata()
		if meta, ok := pluginMetadata[providerID]; ok && meta.LoadTime == api.LoadTimeLazy {
			// We need a way to reach the loader from here...
			// For Milestone 1, we assume the kernel handles lazy-loading naturally via HandleMessageSync
			// but HandleMessageSync doesn't know about the custom mounts we need for discovery.
			// So we manually trigger the loader here if it's a lazy-load.
			if loader, ok := kernel.GetLoader(providerID); ok {
				_, err := loader.LoadPluginWithMounts(ctx, providerID, mounts)
				if err != nil {
					return fmt.Errorf("failed to lazy-load provider with mounts: %w", err)
				}
			}
		}
	}

	discoverPayload, _ := json.Marshal(map[string]any{
		"root":     "/work",
		"base_id":  baseID,
		"metadata": metadata,
	})

	resp, err := kernel.HandleMessageSync(ctx, api.Message{
		ID:        "discover-" + fmt.Sprint(time.Now().UnixNano()),
		Type:      api.TypeRequest,
		Sender:    b.ID(),
		Target:    providerID,
		Method:    "base:discover",
		Payload:   discoverPayload,
		Timestamp: time.Now().Unix(),
	})

	if err != nil {
		return fmt.Errorf("discovery request failed: %w", err)
	}

	var manifest api.BaseManifest
	if err := json.Unmarshal(resp.Payload, &manifest); err != nil {
		return fmt.Errorf("failed to parse base manifest from provider: %w", err)
	}

	base := &BaseInstance{
		ID:        baseID,
		Manifest:  &manifest,
		Metadata:  metadata,
		Plugins:   make(map[string]api.Plugin),
		CreatedAt: time.Now(),
	}

	b.mu.Lock()
	b.bases[baseID] = base
	b.mu.Unlock()

	// 3. Realize Plugins from Manifest
	for pluginID, config := range manifest.Plugins {
		if err := b.realizePlugin(ctx, base, pluginID, config); err != nil {
			b.logger.Warn("failed to realize plugin for base", "base_id", baseID, "plugin_id", pluginID, "error", err)
		}
	}

	b.logger.Info("base activated successfully", "id", baseID, "plugins", len(manifest.Plugins))

	// 4. Emit base:ready event
	b.kernel.RouteMessage(ctx, api.Message{
		ID:        "evt-base-ready-" + baseID,
		Type:      api.TypeEvent,
		Sender:    b.ID(),
		Target:    "*",
		Method:    "base:ready",
		Payload:   []byte(`{"id":"` + baseID + `"}`),
		Timestamp: time.Now().Unix(),
	})

	return nil
}

func (b *BaseManager) realizePlugin(ctx context.Context, base *BaseInstance, pluginID string, config json.RawMessage) error {
	// Check instance pattern
	// In a real implementation, we would check the official metadata.
	// For Milestone 1, we assume multi-instance if it's not a core service.

	// Composite ID for isolation
	instanceID := fmt.Sprintf("%s:%s", base.ID, pluginID)

	// 1. Check if plugin is already loaded as a global/mono instance
	if _, ok := b.kernel.GetPlugin(pluginID); ok {
		// If it's a Mono instance, we just send it the config
		b.logger.Debug("routing config to mono-instance plugin", "base_id", base.ID, "plugin_id", pluginID)
		b.kernel.RouteMessage(ctx, api.Message{
			ID:        "cfg-" + instanceID,
			Type:      api.TypeRequest,
			Sender:    b.ID(),
			Target:    pluginID,
			Method:    "config:update",
			Payload:   config,
			Timestamp: time.Now().Unix(),
			BaseID:    base.ID,
		})
		return nil
	}

	// 2. Attempt to resolve and load WASM
	// For Milestone 1, we expect the plugin to be available for lazy-loading
	// via its global ID. Lazy-loading will then create the instance.

	// Note: The current lazy-load logic in Kernel only supports one instance per PluginID.
	// We need a more flexible way to load multiple instances of the same WASM path.

	b.logger.Info("realizing multi-instance plugin", "base_id", base.ID, "plugin_id", pluginID)

	// Since we haven't refactored the whole WASM loader yet, we'll route a mock config
	// to the global ID if it exists, or emit a warning if not.
	// (Milestone 2/3 will handle the actual auto-loading into isolated instances)

	return nil
}

func (b *BaseManager) RegisterProvider(id string, provider api.BaseProvider) {
	// No-op for now; the BaseManager now delegates discovery to structured capabilities
}

func (b *BaseManager) errorResponse(msg api.Message, err error) api.Message {
	return api.Message{
		ID:        msg.ID + "-resp",
		Type:      api.TypeResponse,
		Sender:    b.ID(),
		Target:    msg.Sender,
		Payload:   []byte(`{"error":"` + err.Error() + `"}`),
		Timestamp: time.Now().Unix(),
	}
}

func (b *BaseManager) successResponse(msg api.Message) api.Message {
	return api.Message{
		ID:        msg.ID + "-resp",
		Type:      api.TypeResponse,
		Sender:    b.ID(),
		Target:    msg.Sender,
		Payload:   []byte(`{"status":"ok"}`),
		Timestamp: time.Now().Unix(),
	}
}

func (b *BaseManager) Shutdown(ctx context.Context) error {
	return nil
}
