package kernel

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jnesbitt/alloy-go/api"
)

// Kernel is the core component that manages plugins and message routing.
type Kernel struct {
	logger *slog.Logger
	mu     sync.RWMutex
	
	// plugins maps plugin IDs to their instances
	plugins map[string]Plugin
	
	// frontends maps connection IDs to their message channels
	frontends map[string]chan<- api.Message
	
	// stopCh is used to signal the kernel to shut down
	stopCh chan struct{}
}

// Plugin defines the interface for backend extensions.
type Plugin interface {
	ID() string
	HandleMessage(ctx context.Context, msg api.Message) (api.Message, error)
	Shutdown(ctx context.Context) error
}

// New creates a new instance of the Alloy Kernel.
func New(logger *slog.Logger) *Kernel {
	return &Kernel{
		logger:    logger,
		plugins:   make(map[string]Plugin),
		frontends: make(map[string]chan<- api.Message),
		stopCh:    make(chan struct{}),
	}
}

// Start initializes the kernel services.
func (k *Kernel) Start(ctx context.Context) error {
	k.logger.Info("starting alloy kernel")
	return nil
}

// Stop shuts down the kernel and all active plugins.
func (k *Kernel) Stop(ctx context.Context) error {
	k.logger.Info("stopping alloy kernel")
	close(k.stopCh)
	return nil
}

// RegisterPlugin attaches a plugin to the kernel.
func (k *Kernel) RegisterPlugin(p Plugin) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.plugins[p.ID()] = p
	k.logger.Info("plugin registered", "plugin_id", p.ID())
}

// RouteMessage handles the delivery of a message to its intended target.
func (k *Kernel) RouteMessage(ctx context.Context, msg api.Message) {
	k.logger.Debug("routing message", "id", msg.ID, "method", msg.Method, "target", msg.Target)

	k.mu.RLock()
	plugin, isPlugin := k.plugins[msg.Target]
	frontendChan, isFrontend := k.frontends[msg.Target]
	k.mu.RUnlock()

	if isPlugin {
		go func() {
			resp, err := plugin.HandleMessage(ctx, msg)
			if err != nil {
				k.logger.Error("plugin error", "plugin_id", msg.Target, "error", err)
				return
			}
			k.RouteMessage(ctx, resp)
		}()
		return
	}

	if isFrontend {
		select {
		case frontendChan <- msg:
			k.logger.Debug("message delivered to frontend", "target", msg.Target)
		case <-ctx.Done():
			k.logger.Warn("context cancelled while delivering to frontend", "target", msg.Target)
		default:
			k.logger.Error("frontend buffer full or closed", "target", msg.Target)
		}
		return
	}

	k.logger.Warn("message target not found", "target", msg.Target)
}

// RegisterFrontend registers a frontend's response channel.
func (k *Kernel) RegisterFrontend(id string, ch chan<- api.Message) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.frontends[id] = ch
	k.logger.Info("frontend registered", "frontend_id", id)
}
