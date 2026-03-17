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
	k.plugins[p.ID()] = p
	k.mu.Unlock()

	k.logger.Info("plugin registered", "plugin_id", p.ID())

	// Emit registration event
	caps, _ := json.Marshal(p.Capabilities())
	k.RouteMessage(context.Background(), api.Message{
		ID:        "event-reg-" + p.ID(),
		Type:      api.TypeEvent,
		Sender:    "kernel",
		Target:    "plugin-events",
		Method:    "publish",
		Payload:   []byte(`{"topic":"component:registered","data":{"id":"` + p.ID() + `","type":"plugin","capabilities":` + string(caps) + `}}`),
		Timestamp: time.Now().Unix(),
	})

	k.publishAuditEvent(context.Background(), api.Message{Sender: "system", Target: p.ID()}, "plugin_register", "success")
}

// RouteMessage handles the delivery of a message to its intended target.
func (k *Kernel) RouteMessage(ctx context.Context, msg api.Message) {
	k.logger.Debug("routing message", "id", msg.ID, "method", msg.Method, "target", msg.Target)

	k.mu.RLock()
	plugin, isPlugin := k.plugins[msg.Target]
	frontendChan, isFrontend := k.frontends[msg.Target]
	k.mu.RUnlock()

	// Emit auditing event for core routing (if enabled)
	k.publishAuditEvent(ctx, msg, "route", "processed")

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
			if resp.ID != "" || resp.Target != "" {
				k.RouteMessage(ctx, resp)
			}
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
	k.frontends[id] = ch
	k.mu.Unlock()

	k.logger.Info("frontend registered", "frontend_id", id)

	// Emit registration event
	k.RouteMessage(context.Background(), api.Message{
		ID:        "event-reg-" + id,
		Type:      api.TypeEvent,
		Sender:    "kernel",
		Target:    "plugin-events",
		Method:    "publish",
		Payload:   []byte(`{"topic":"component:registered","data":{"id":"` + id + `","type":"frontend"}}`),
		Timestamp: time.Now().Unix(),
	})
}

func (k *Kernel) handleInternalMessage(ctx context.Context, msg api.Message) {
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
	case "audit":
		// Handle audit log request (e.g., from an external auditor plugin)
		k.publishAuditEvent(ctx, msg, "audit_request", "authorized")
	case "stop":
		k.logger.Info("stop request received via internal channel")
		k.Stop(ctx)
	default:
		k.logger.Warn("unknown internal method", "method", msg.Method)
	}
}

// StopCh returns the shutdown signal channel.
func (k *Kernel) StopCh() <-chan struct{} {
	return k.stopCh
}

func (k *Kernel) publishAuditEvent(ctx context.Context, msg api.Message, action, status string) {
	// Avoid recursive auditing:
	// 1. Don't audit messages targeting the event/audit system itself
	// 2. Don't audit messages sent by the kernel (internal events/responses)
	if msg.Target == "plugin-events" || msg.Sender == "kernel" {
		return
	}

	// Traditional structured audit log (if configured)
	if k.audit != nil {
		k.audit.Log(audit.Entry{
			Actor:  msg.Sender,
			Action: action,
			Target: msg.Target,
			Status: status,
			Details: map[string]any{
				"method": msg.Method,
				"type":   msg.Type,
			},
		})
	}

	// Modern Event-driven audit log
	// We use a non-blocking go-routine to avoid routing cycles or delays
	go func() {
		details, _ := json.Marshal(map[string]any{
			"actor":  msg.Sender,
			"action": action,
			"target": msg.Target,
			"status": status,
			"method": msg.Method,
		})

		k.RouteMessage(context.Background(), api.Message{
			ID:        "audit-" + time.Now().Format("150405.000"),
			Type:      api.TypeEvent,
			Sender:    "kernel",
			Target:    "plugin-events",
			Method:    "publish",
			Payload:   []byte(`{"topic":"system:audit","data":` + string(details) + `}`),
			Timestamp: time.Now().Unix(),
		})
	}()
}
