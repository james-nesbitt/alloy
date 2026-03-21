package wit

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jnesbitt/alloy-go/pkg/storage"
	"github.com/jnesbitt/alloy-go/pkg/wasm/bindings/guest"
)

// WITRuntime manages the WASM runtime environment using WIT bindings.
// This is a preliminary implementation that will evolve as we adopt WIT.
type WITRuntime struct {
	runtime    *guest.AlloyGuest
	logger     *slog.Logger
	kv         storage.StateStore
	dataDir    string
	plugins    map[string]*WITInstance
	mu         sync.RWMutex
	routerFn   func(ctx context.Context, msg guest.AlloyMessage)
	callFn     func(ctx context.Context, msg guest.AlloyMessage) (guest.AlloyMessage, error)
}

// WITInstance represents a WASM plugin instance using WIT bindings.
type WITInstance struct {
	id           string
	ctx          context.Context
	cancel       context.CancelFunc
	instance     *guest.AlloyInstance
	logger       *slog.Logger
	status       guest.AlloyStatus
	capabilities []guest.AlloyCapability
	msgChan      chan guest.AlloyMessage
	respChan     chan guest.AlloyMessage
}

// NewWITRuntime creates a new WIT-based WASM runtime.
func NewWITRuntime(
	logger *slog.Logger,
	kv storage.StateStore,
	dataDir string,
	router func(ctx context.Context, msg guest.AlloyMessage),
	call func(ctx context.Context, msg guest.AlloyMessage) (guest.AlloyMessage, error),
) (*WITRuntime, error) {
	// Initialize the WIT guest bindings
	guestRuntime, err := guest.NewAlloyGuest()
	if err != nil {
		return nil, err
	}

	return &WITRuntime{
		runtime:  guestRuntime,
		logger:   logger,
		kv:       kv,
		dataDir:  dataDir,
		plugins:  make(map[string]*WITInstance),
		routerFn: router,
		callFn:   call,
	}, nil
}

// LoadWITPlugin instantiates a WASM plugin using WIT bindings.
func (r *WITRuntime) LoadWITPlugin(
	ctx context.Context,
	id string,
	wasmBytes []byte,
	caps []guest.AlloyCapability,
) (*WITInstance, error) {
	// In a real implementation, we would:
	// 1. Compile the WASM module
	// 2. Instantiate it with the WIT bindings
	// 3. Set up the instance with proper context and channels
	// 4. Initialize the plugin with its capabilities

	instCtx, instCancel := context.WithCancel(ctx)

	// Create a mock instance for now
	instance := &WITInstance{
		id:           id,
		ctx:          instCtx,
		cancel:       instCancel,
		logger:       r.logger,
		status:       guest.AlloyStatusRunning(),
		capabilities: caps,
		msgChan:      make(chan guest.AlloyMessage, 32),
		respChan:     make(chan guest.AlloyMessage, 32),
	}

	// Register the plugin
	r.mu.Lock()
	r.plugins[id] = instance
	r.mu.Unlock()

	// Initialize the plugin
	instance.instance.AlloyInit(id, caps)

	return instance, nil
}

// Example of how the host would implement WIT interface functions
// These would be registered with the WIT runtime

// alloyLog implements the WIT log function
func (r *WITRuntime) alloyLog(level string, message string) {
	switch level {
	case "debug":
		r.logger.Debug(message)
	case "info":
		r.logger.Info(message)
	case "warn":
		r.logger.Warn(message)
	case "error":
		r.logger.Error(message)
	default:
		r.logger.Info(message, "level", level)
	}
}

// alloyKVSet implements the WIT kv-set function
func (r *WITRuntime) alloyKVSet(key string, value []byte) bool {
	pluginID := getCurrentPluginID() // Would get this from WIT context
	if err := r.kv.Set(pluginID, key, value); err != nil {
		return false
	}
	return true
}

// alloyRouteMessage implements the WIT route-message function
func (r *WITRuntime) alloyRouteMessage(msg guest.AlloyMessage) {
	pluginID := getCurrentPluginID() // Would get this from WIT context
	msg.Sender = pluginID
	r.routerFn(context.Background(), msg)
}

// getCurrentPluginID is a placeholder for getting the current plugin ID from WIT context
func getCurrentPluginID() string {
	// In a real implementation, this would get the plugin ID from the WIT context
	return "current-plugin"
}