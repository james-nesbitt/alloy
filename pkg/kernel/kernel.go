package kernel

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/security/audit"
)

// Kernel is the core component that manages plugins and message routing.
type Kernel struct {
	logger *slog.Logger
	audit  *audit.Logger
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
	Capabilities() []api.Capability
	HandleMessage(ctx context.Context, msg api.Message) (api.Message, error)
	Shutdown(ctx context.Context) error
}

// New creates a new instance of the Alloy Kernel.
func New(logger *slog.Logger, audit *audit.Logger) *Kernel {
	return &Kernel{
		logger:    logger,
		audit:     audit,
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
	if k.audit != nil {
		k.audit.Log(audit.Entry{Actor: "system", Action: "plugin_register", Target: p.ID(), Status: "success"})
	}
}

// RouteMessage handles the delivery of a message to its intended target.
func (k *Kernel) RouteMessage(ctx context.Context, msg api.Message) {
	k.logger.Debug("routing message", "id", msg.ID, "method", msg.Method, "target", msg.Target)

	k.mu.RLock()
	plugin, isPlugin := k.plugins[msg.Target]
	frontendChan, isFrontend := k.frontends[msg.Target]
	k.mu.RUnlock()

	if k.audit != nil {
		k.audit.Log(audit.Entry{
			Actor:  msg.Sender,
			Action: "route",
			Target: msg.Target,
			Status: "processed",
			Details: map[string]any{
				"method": msg.Method,
				"type":   msg.Type,
			},
		})
	}

	if msg.Target == "kernel" || msg.Target == "system" {
		k.handleInternalMessage(ctx, msg)
		return
	}

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

func (k *Kernel) handleInternalMessage(ctx context.Context, msg api.Message) {
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
	case "discover":
		k.mu.RLock()
		type registration struct {
			ID           string           `json:"id"`
			Capabilities []api.Capability `json:"capabilities,omitempty"`
			Type         string           `json:"type"`
		}
		var targets []registration
		for id, p := range k.plugins {
			targets = append(targets, registration{
				ID:           id,
				Capabilities: p.Capabilities(),
				Type:         "plugin",
			})
		}
		for id := range k.frontends {
			targets = append(targets, registration{
				ID:   id,
				Type: "frontend",
			})
		}
		k.mu.RUnlock()

		payload, _ := json.Marshal(map[string]any{
			"targets": targets,
		})
		resp := api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    "kernel",
			Target:    msg.Sender,
			Method:    "discover",
			Payload:   payload,
			Timestamp: time.Now().Unix(),
		}
		k.RouteMessage(ctx, resp)
	default:
		k.logger.Warn("unknown internal method", "method", msg.Method)
	}
}
