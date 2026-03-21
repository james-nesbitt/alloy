package kernel

import (
	"context"
	"encoding/json"
	"fmt"
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

	// plugins maps plugin IDs to their active instances
	plugins map[string]api.Plugin

	// metadata maps plugin IDs to their metadata (for lazy loading)
	metadata map[string]api.PluginMetadata

	// loaders maps plugin IDs to a loader that can instantiate them
	loaders map[string]api.PluginLoader

	// frontends maps connection IDs to their message channels
	frontends map[string]chan<- api.Message

	// interceptors is a list of components that can filter or modify messages before delivery
	interceptors []api.Interceptor

	// loading keeps track of plugins currently being lazy-loaded
	loading map[string]chan struct{}

	// stopCh is used to signal the kernel to shut down
	stopCh chan struct{}

	telemetry *Telemetry
}

// New creates a new instance of the Alloy Kernel.
func New(logger *slog.Logger) *Kernel {
	tel, _ := initTelemetry()
	return &Kernel{
		logger:       logger,
		plugins:      make(map[string]api.Plugin),
		metadata:     make(map[string]api.PluginMetadata),
		loaders:      make(map[string]api.PluginLoader),
		frontends:    make(map[string]chan<- api.Message),
		interceptors: make([]api.Interceptor, 0),
		loading:      make(map[string]chan struct{}),
		stopCh:       make(chan struct{}),
		tracer:       otel.Tracer(tracerName),
		telemetry:    tel,
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

// RegisterMetadata describes a plugin to the kernel without loading it yet.
func (k *Kernel) RegisterMetadata(info api.PluginMetadata, loader api.PluginLoader) {
	k.mu.Lock()
	k.metadata[info.ID] = info
	k.loaders[info.ID] = loader
	k.mu.Unlock()

	k.logger.Info("plugin metadata registered", "plugin_id", info.ID, "load_time", info.LoadTime)

	go func() {
		// Emit registration event so CommandManager knows about it even before load
		metaData, _ := json.Marshal(map[string]any{
			"id":           info.ID,
			"type":         "plugin-meta",
			"capabilities": info.Capabilities,
		})
		
		publishPayload, _ := json.Marshal(map[string]any{
			"topic": "component:registered",
			"data":  json.RawMessage(metaData),
		})
		
		systemCtx := context.WithValue(context.Background(), auditContextKey, true)
		systemCtx = context.WithValue(systemCtx, skipInterceptorsKey, true)

		k.logger.Debug("emitting component:registered event", "plugin_id", info.ID)
		k.RouteMessage(systemCtx, api.Message{
			ID:        "event-reg-meta-" + info.ID,
			Type:      api.TypeEvent,
			Sender:    "kernel",
			Target:    "plugin-events",
			Method:    "publish",
			Payload:   publishPayload,
			Timestamp: time.Now().Unix(),
		})
	}()
}

// BootPlugins triggers the manual instantiation of all plugins configured to load at boot.
func (k *Kernel) BootPlugins(ctx context.Context) error {
	k.mu.RLock()
	var boot []string
	for id, info := range k.metadata {
		if info.LoadTime == api.LoadTimeBoot {
			if _, active := k.plugins[id]; !active {
				boot = append(boot, id)
			}
		}
	}
	k.mu.RUnlock()

	for _, id := range boot {
		k.mu.RLock()
		loader, hasLoader := k.loaders[id]
		k.mu.RUnlock()

		if hasLoader {
			k.logger.Info("booting plugin", "plugin_id", id)
			p, err := loader.LoadPlugin(ctx, id)
			if err != nil {
				return fmt.Errorf("failed to boot plugin %s: %w", id, err)
			}
			k.RegisterPlugin(p)
		}
	}
	return nil
}

// RegisterPlugin attaches an active plugin to the kernel.
func (k *Kernel) RegisterPlugin(p api.Plugin) {
	k.mu.Lock()
	if _, exists := k.plugins[p.ID()]; !exists {
		k.telemetry.PluginCountChange(context.Background(), 1)
	}
	k.plugins[p.ID()] = p
	// Automatically register if it implements Interceptor
	if i, ok := p.(api.Interceptor); ok {
		k.interceptors = append(k.interceptors, i)
		k.logger.Info("interceptor registered", "plugin_id", p.ID())
	}

	// Update metadata with actual capabilities once fully loaded
	existingMeta, ok := k.metadata[p.ID()]
	if !ok || len(existingMeta.Capabilities) == 0 {
		k.metadata[p.ID()] = api.PluginMetadata{
			ID:           p.ID(),
			Capabilities: p.Capabilities(),
			LoadTime:     api.LoadTimeBoot, // Active plugins are considered "booted"
		}
	}
	k.mu.Unlock()

	k.logger.Info("plugin registered and active", "plugin_id", p.ID())

	// Handle IAM enforcement natively
	if p.ID() == "plugin-iam" {
		k.logger.Info("IAM plugin detected, enabling RBAC enforcement")
		// In a real system, we might promote IAM to a first-class interceptor
	}

	go func() {
		// Emit registration event
		caps := p.Capabilities()
		capsData, _ := json.Marshal(caps)
		// Use non-auditing/intercepting context for system-level events
		systemCtx := context.WithValue(context.Background(), auditContextKey, true)
		systemCtx = context.WithValue(systemCtx, skipInterceptorsKey, true)

		k.RouteMessage(systemCtx, api.Message{
			ID:        "event-reg-" + p.ID(),
			Type:      api.TypeEvent,
			Sender:    "kernel",
			Target:    "plugin-events",
			Method:    "publish",
			Payload:   []byte(`{"topic":"component:registered","data":{"id":"` + p.ID() + `","type":"plugin","capabilities":` + string(capsData) + `}}`),
			Timestamp: time.Now().Unix(),
		})

		k.publishAuditEvent(systemCtx, api.Message{Sender: "system", Target: p.ID()}, "plugin_register", "success")
	}()
}

func (k *Kernel) deliverToPlugin(ctx context.Context, p api.Plugin, msg api.Message) {
	go func(plugin api.Plugin, m api.Message, c context.Context) {
		childCtx, childSpan := k.tracer.Start(c, "plugin.HandleMessage",
			trace.WithAttributes(attribute.String("alloy.plugin.id", plugin.ID())))
		defer childSpan.End()

		resp, err := plugin.HandleMessage(childCtx, m)
		if err != nil {
			k.logger.Error("plugin error", "plugin_id", m.Target, "error", err)
			childSpan.RecordError(err)

			// Status tracking for crash-aware plugins
			type statusAware interface{ IsCrashed() bool }
			if sa, ok := p.(statusAware); ok && sa.IsCrashed() {
				k.publishCrashEvent(m.Target, err.Error())
			}
			return
		}
		if resp.ID != "" || resp.Target != "" {
			k.RouteMessage(childCtx, resp)
		}
	}(p, msg, ctx)
}

// RouteMessage handles the delivery of a message to its intended target.
func (k *Kernel) RouteMessage(ctx context.Context, msg api.Message) {
	// Start OpenTelemetry span
	ctx, span := k.tracer.Start(ctx, "kernel.RouteMessage",
		trace.WithAttributes(msg.ToSpanAttributes()...))
	defer span.End()

	k.logger.Debug("routing message", "id", msg.ID, "sender", msg.Sender, "method", msg.Method, "target", msg.Target)
	k.telemetry.RecordMessage(ctx, msg.Sender, msg.Target, msg.Method)

	// Pre-Route Interception
	if ctx.Value(skipInterceptorsKey) == nil {
		k.mu.RLock()
		interceptors := k.interceptors
		iam, hasIAM := k.plugins["plugin-iam"]
		k.mu.RUnlock()

		// Core RBAC Enforcement via plugin-iam
		if hasIAM && msg.Sender != "system" && msg.Sender != "plugin-iam" && msg.Method != "check" && msg.Type == api.TypeRequest {
			// Ask IAM for permission
			checkPayload, _ := json.Marshal(map[string]string{
				"actor":  msg.Actor,
				"target": msg.Target,
				"method": msg.Method,
			})
			iamCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
			defer cancel()
			
			// Call the plugin handler directly to avoid recursive routing
			resp, err := iam.HandleMessage(iamCtx, api.Message{
				ID:      "iam-check-" + msg.ID,
				Type:    api.TypeRequest,
				Sender:  "kernel",
				Target:  "plugin-iam",
				Method:  "check",
				Payload: checkPayload,
			})
			
			if err != nil {
				k.logger.Error("IAM check failed", "error", err)
				return
			}
			
			var result struct {
				Allowed bool `json:"allowed"`
			}
			if err := json.Unmarshal(resp.Payload, &result); err == nil && !result.Allowed {
				k.logger.Warn("IAM: authorization denied", "actor", msg.Actor, "target", msg.Target, "method", msg.Method)
				
				// Optional: Send access denied response back to sender
				k.RouteMessage(context.WithValue(ctx, skipInterceptorsKey, true), api.Message{
					ID:      msg.ID + "-error",
					Type:    api.TypeResponse,
					Sender:  "kernel",
					Target:  msg.Sender,
					Payload: []byte(`{"error":"unauthorized"}`),
				})
				return
			}
		}

		for _, interceptor := range interceptors {
			newMsg, allow, err := interceptor.PreRoute(ctx, msg)
			if err != nil {
				k.logger.Error("interceptor error", "error", err, "target", msg.Target)
				k.telemetry.RecordError(ctx, msg.Target, "interceptor_fail")
				span.RecordError(err)
				return
			}
			if !allow {
				k.logger.Warn("routing denied by interceptor", "sender", msg.Sender, "target", msg.Target)
				k.telemetry.RecordError(ctx, msg.Target, "auth_denied")
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
	_, hasMetadata := k.metadata[msg.Target]
	loader, hasLoader := k.loaders[msg.Target]
	k.mu.RUnlock()

	// 1. Audit core routing
	if ctx.Value(auditContextKey) == nil {
		k.publishAuditEvent(ctx, msg, "route", "processed")
	}

	// 2. Handle internal messages
	if msg.Target == "kernel" || msg.Target == "system" {
		k.handleInternalMessage(ctx, msg)
		return
	}

	// 3. Handle active plugins
	if isPlugin {
		k.deliverToPlugin(ctx, plugin, msg)
		return
	}

	// 4. Handle lazy loading
	if !isPlugin && hasMetadata && hasLoader {
		go func() {
			k.mu.Lock()
			if loadCh, inProgress := k.loading[msg.Target]; inProgress {
				k.mu.Unlock()
				<-loadCh // Wait for the in-progress load
				// After waiting, check again if it was successful
				k.mu.RLock()
				plugin, isNowActive := k.plugins[msg.Target]
				k.mu.RUnlock()
				if isNowActive {
					k.deliverToPlugin(ctx, plugin, msg)
				} else {
					// Load must have failed - previous load would have reported error
					// but we don't know the exact error here. Let's just retry or report generic error.
					k.logger.Warn("waiting for lazy-load failed: target not active")
				}
				return
			}

			// First one here: start the load
			loadCh := make(chan struct{})
			k.loading[msg.Target] = loadCh
			k.mu.Unlock()

			defer func() {
				k.mu.Lock()
				delete(k.loading, msg.Target)
				close(loadCh)
				k.mu.Unlock()
			}()

			k.logger.Info("lazy loading plugin on message request", "plugin_id", msg.Target)
			loadCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			
			p, err := loader.LoadPlugin(loadCtx, msg.Target)
			if err == nil {
				k.RegisterPlugin(p)
				k.deliverToPlugin(ctx, p, msg)
			} else {
				k.logger.Error("failed to lazy-load plugin", "plugin_id", msg.Target, "error", err)
				// Notify the sender that the lazy-load failed
				k.logger.Debug("sending lazy-load failure response", "target", msg.Sender, "original_id", msg.ID)
				k.RouteMessage(context.WithValue(context.Background(), skipInterceptorsKey, true), api.Message{
					ID:        msg.ID + "-resp",
					Type:      api.TypeResponse,
					Sender:    "kernel",
					Target:    msg.Sender,
					Payload:   []byte(`{"error":"failed_to_load_plugin","details":"` + err.Error() + `"}`),
					Timestamp: time.Now().Unix(),
				})
			}
		}()
		return
	}

	// 5. Handle frontends
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

func (k *Kernel) publishCrashEvent(id, err string) {
	k.RouteMessage(context.WithValue(context.Background(), skipInterceptorsKey, true), api.Message{
		ID:        "evt-crash-" + id + "-" + fmt.Sprint(time.Now().UnixNano()),
		Type:      api.TypeEvent,
		Sender:    "kernel",
		Target:    "plugin-events",
		Method:    "publish",
		Payload:   []byte(`{"topic":"plugin:crashed","data":{"id":"` + id + `","error":"` + err + `"}}`),
		Timestamp: time.Now().Unix(),
	})
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
