package kernel

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	auditContextKey      = "alloy.no_audit"
	skipInterceptorsKey  = "alloy.skip_interceptors"
	tracerName           = "alloy-kernel"
)

// Kernel is the core component that manages plugins and message routing.
type Kernel struct {
	logger *slog.Logger
	mu     sync.RWMutex
	tracer trace.Tracer

	// plugins maps plugin IDs to their instances
	plugins map[string]api.Plugin

	// frontends maps connection IDs to their message channels
	frontends map[string]chan<- api.Message

	// interceptors is a list of components that can filter or modify messages before delivery
	interceptors []api.Interceptor

	// stopCh is used to signal the kernel to shut down
	stopCh chan struct{}
}

// New creates a new instance of the Alloy Kernel.
func New(logger *slog.Logger) *Kernel {
	return &Kernel{
		logger:       logger,
		plugins:      make(map[string]api.Plugin),
		frontends:    make(map[string]chan<- api.Message),
		interceptors: make([]api.Interceptor, 0),
		stopCh:       make(chan struct{}),
		tracer:       otel.Tracer(tracerName),
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
func (k *Kernel) RegisterPlugin(p api.Plugin) {
	k.mu.Lock()
	k.plugins[p.ID()] = p
	// Automatically register if it implements Interceptor
	if i, ok := p.(api.Interceptor); ok {
		k.interceptors = append(k.interceptors, i)
		k.logger.Info("interceptor registered", "plugin_id", p.ID())
	}
	k.mu.Unlock()

	k.logger.Info("plugin registered", "plugin_id", p.ID())

	// Emit registration event
	caps, _ := json.Marshal(p.Capabilities())
	// Use non-auditing/intercepting context for system-level events
	systemCtx := context.WithValue(context.Background(), auditContextKey, true)
	systemCtx = context.WithValue(systemCtx, skipInterceptorsKey, true)

	k.RouteMessage(systemCtx, api.Message{
		ID:        "event-reg-" + p.ID(),
		Type:      api.TypeEvent,
		Sender:    "kernel",
		Target:    "plugin-events",
		Method:    "publish",
		Payload:   []byte(`{"topic":"component:registered","data":{"id":"` + p.ID() + `","type":"plugin","capabilities":` + string(caps) + `}}`),
		Timestamp: time.Now().Unix(),
	})

	k.publishAuditEvent(systemCtx, api.Message{Sender: "system", Target: p.ID()}, "plugin_register", "success")
}

// RouteMessage handles the delivery of a message to its intended target.
func (k *Kernel) RouteMessage(ctx context.Context, msg api.Message) {
	// Start OpenTelemetry span
	ctx, span := k.tracer.Start(ctx, "kernel.RouteMessage",
		trace.WithAttributes(msg.ToSpanAttributes()...))
	defer span.End()

	k.logger.Debug("routing message", "id", msg.ID, "sender", msg.Sender, "method", msg.Method, "target", msg.Target)

	// Pre-Route Interception
	if ctx.Value(skipInterceptorsKey) == nil {
		k.mu.RLock()
		interceptors := k.interceptors
		k.mu.RUnlock()

		for _, interceptor := range interceptors {
			newMsg, allow, err := interceptor.PreRoute(ctx, msg)
			if err != nil {
				k.logger.Error("interceptor error", "error", err, "target", msg.Target)
				span.RecordError(err)
				return
			}
			if !allow {
				k.logger.Warn("routing denied by interceptor", "sender", msg.Sender, "target", msg.Target)
				span.SetAttributes(attribute.Bool("alloy.msg.allowed", false))
				return
			}
			msg = newMsg
		}
	}
	span.SetAttributes(attribute.Bool("alloy.msg.allowed", true))

	k.mu.RLock()
	plugin, isPlugin := k.plugins[msg.Target]
	frontendChan, isFrontend := k.frontends[msg.Target]
	k.mu.RUnlock()

	// Emit auditing event for core routing (if enabled)
	if ctx.Value(auditContextKey) == nil {
		k.publishAuditEvent(ctx, msg, "route", "processed")
	}

	if msg.Target == "kernel" || msg.Target == "system" {
		k.handleInternalMessage(ctx, msg)
		return
	}

	if isPlugin {
		go func(p api.Plugin, m api.Message, c context.Context) {
			childCtx, childSpan := k.tracer.Start(c, "plugin.HandleMessage",
				trace.WithAttributes(attribute.String("alloy.plugin.id", p.ID())))
			defer childSpan.End()

			resp, err := p.HandleMessage(childCtx, m)
			if err != nil {
				k.logger.Error("plugin error", "plugin_id", m.Target, "error", err)
				childSpan.RecordError(err)
				return
			}
			if resp.ID != "" || resp.Target != "" {
				k.RouteMessage(childCtx, resp)
			}
		}(plugin, msg, ctx)
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
	if existing, ok := k.frontends[id]; ok && existing == ch {
		k.mu.Unlock()
		return
	}
	k.frontends[id] = ch
	k.mu.Unlock()

	k.logger.Info("frontend registered", "frontend_id", id)

	// Emit registration event
	systemCtx := context.WithValue(context.Background(), auditContextKey, true)
	systemCtx = context.WithValue(systemCtx, skipInterceptorsKey, true)

	k.RouteMessage(systemCtx, api.Message{
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
	return
	// Avoid recursive auditing and system noise:
	if msg.Target == "plugin-events" || msg.Target == "plugin-logger" || msg.Target == "plugin-kv" ||
		msg.Sender == "kernel" || msg.Sender == "plugin-events" || msg.Sender == "plugin-logger" || msg.Sender == "plugin-kv" ||
		msg.Sender == "system" || msg.Sender == "ipc-server" ||
		msg.Method == "system:audit" || msg.Method == "component:registered" {
		return
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

		// Use a explicit context to prevent audit loops or interception at the routing level
		auditCtx := context.WithValue(context.Background(), auditContextKey, true)
		auditCtx = context.WithValue(auditCtx, skipInterceptorsKey, true)

		auditMsg := api.Message{
			ID:        "audit-" + time.Now().Format("150405.000"),
			Type:      api.TypeEvent,
			Sender:    "kernel",
			Target:    "plugin-events",
			Method:    "publish",
			Payload:   []byte(`{"topic":"system:audit","data":` + string(details) + `}`),
			Timestamp: time.Now().Unix(),
		}

		// Propagate trace context to audit log message if present
		if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
			auditMsg.InjectSpanContext(span.SpanContext())
		}

		k.RouteMessage(auditCtx, auditMsg)
	}()
}
