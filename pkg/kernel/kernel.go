package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
	"github.com/james-nesbitt/alloy/pkg/storage/history"

	"github.com/james-nesbitt/alloy/pkg/wasm"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	auditContextKey     = "alloy.no_audit"
	skipInterceptorsKey = "alloy.skip_interceptors"
	tracerName          = "alloy-kernel"
)

// Kernel is the core component that manages plugins and message routing.
type Kernel struct {
	logger   *slog.Logger
	mu       sync.RWMutex
	stopCh   chan struct{}
	stopOnce sync.Once
	tracer   trace.Tracer

	// active plugins, metadata, loaders, and frontends
	plugins   map[string]api.Plugin
	metadata  map[string]api.PluginMetadata
	loaders   map[string]api.PluginLoader
	frontends map[string]chan<- api.Message
	loading   map[string]chan struct{}

	// context for background tasks
	ctx    context.Context
	cancel context.CancelFunc

	// WASM environment
	wasmManager *wasm.Manager
	storage     storage.StateStore
	dataDir     string
	intents     *IntentBroker
	history     *history.Store
	eventCh     chan api.Message

	// telemetry
	telemetry *Telemetry

	// integrated core services
	events     *EventManager
	iam        *IdentityManager
	commands   *CommandManager
	health     *HealthManager
	loggerSvc  *LoggerManager
	kv         *KVManager
	storageSvc *StorageManager
	network    *NetworkManager
	cache      *CacheManager
	doc        *DocStore
	buffers    *BufferManager
	widgets    *WidgetManager

	// security configuration
	insecure bool

	// capability management
	capabilityMap map[string]string // capability name -> plugin ID

	// list of components that can filter or modify messages (Legacy interceptors)
	interceptors []api.Interceptor
}

// New creates a new instance of the Alloy Kernel.
func New(logger *slog.Logger, storage storage.StateStore, dataDir string, metricsAddr string) (*Kernel, error) {
	tel, _ := initTelemetry(metricsAddr)
	ctx, cancel := context.WithCancel(context.Background())

	k := &Kernel{
		logger:        logger,
		plugins:       make(map[string]api.Plugin),
		metadata:      make(map[string]api.PluginMetadata),
		loaders:       make(map[string]api.PluginLoader),
		frontends:     make(map[string]chan<- api.Message),
		loading:       make(map[string]chan struct{}),
		stopCh:        make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
		storage:       storage,
		dataDir:       dataDir,
		capabilityMap: make(map[string]string),
		telemetry:     tel,
		tracer:        otel.Tracer(tracerName),
	}

	k.buffers = NewBufferManager(logger, dataDir)
	k.intents = NewIntentBroker(logger, k.RouteMessage, k.QueryLibrarian)

	// Create the WASM manager
	wm, err := wasm.NewManager(logger, storage, dataDir, k.buffers, k.RouteMessage, k.HandleMessageSync)
	if err != nil {
		return nil, err
	}
	k.wasmManager = wm

	k.iam, _ = NewIdentityManager(context.Background(), logger, storage)
	k.RegisterPlugin(k.iam)

	// Initialize and register integrated core services
	k.events = NewEventManager(logger, k.iam)
	k.events.SetRouter(k.RouteMessage)
	k.RegisterPlugin(k.events)

	k.commands = NewCommandManager(logger, k.listRegistrations, k.iam)
	k.RegisterPlugin(k.commands)

	k.health = NewHealthManager(logger)
	k.RegisterPlugin(k.health)

	k.kv = NewKVManager(storage)
	k.RegisterPlugin(k.kv)

	k.storageSvc = NewStorageManager()
	k.RegisterPlugin(k.storageSvc)

	k.network = NewNetworkManager()
	k.RegisterPlugin(k.network)

	k.cache = NewCacheManager()
	k.RegisterPlugin(k.cache)

	k.doc = NewDocStore()
	k.RegisterPlugin(k.doc)

	k.widgets = NewWidgetManager(logger, storage)
	k.RegisterPlugin(k.widgets)

	// Integrated Logger (Auditing)
	auditDir := storage.BaseDir()
	if auditDir != "" {
		auditDir = filepath.Join(filepath.Dir(auditDir), "audit")
	} else {
		// fallback
		auditDir = filepath.Join(dataDir, "audit")
	}
	k.loggerSvc, _ = NewLoggerManager(logger, auditDir)
	if k.loggerSvc != nil {
		k.RegisterPlugin(k.loggerSvc)
		// Subscribe to audit events
		k.RouteMessage(context.Background(), api.Message{
			ID:      "sub-audit",
			Sender:  "logger",
			Target:  "events",
			Method:  "subscribe",
			Payload: []byte(`{"topic":"system:audit"}`),
		})
	}

	// Register WASM manager as a plugin
	k.RegisterPlugin(&wasmManagerPlugin{kernel: k})

	// Start the monitor
	// Start the monitor
	k.wasmManager.StartMonitor(k.ctx, 30*time.Second)

	// Initialize History Store (Phase 11: Event Sourcing)
	eventDir := filepath.Join(dataDir, "events")
	hStore, _ := history.NewStore(eventDir)
	k.history = hStore
	k.eventCh = make(chan api.Message, 10000)

	// Register History Manager to expose history to other plugins
	if k.history != nil {
		hManager := NewHistoryManager(logger, k.history, k.ReplayEvents)
		k.RegisterPlugin(hManager)
		go k.processEventLog()
	}

	return k, nil
}

func (k *Kernel) processEventLog() {
	for {
		select {
		case msg := <-k.eventCh:
			if k.history != nil {
				_, _ = k.history.Append(msg)
			}
		case <-k.stopCh:
			return
		}
	}
}

// SetInsecure disables security enforcement (RBAC, mTLS, etc.) in the kernel.
func (k *Kernel) SetInsecure(insecure bool) {
	k.insecure = insecure
	if insecure {
		k.logger.Warn("running in INSECURE mode - RBAC enforcement disabled")
	}
}

// Start initializes the kernel services.
func (k *Kernel) Start(ctx context.Context) error {
	k.logger.Info("starting alloy kernel")
	return nil
}

// Shutdown gracefully shuts down the kernel.
func (k *Kernel) Shutdown(ctx context.Context) error {
	k.logger.Info("shutting down kernel")
	k.stopOnce.Do(func() {
		close(k.stopCh)
		k.cancel()
	})

	k.mu.Lock()
	plugins := make([]api.Plugin, 0, len(k.plugins))
	for _, plugin := range k.plugins {
		plugins = append(plugins, plugin)
	}
	k.mu.Unlock()

	for _, plugin := range plugins {
		_ = plugin.Shutdown(ctx)
	}

	if k.wasmManager != nil {
		if err := k.wasmManager.Close(ctx); err != nil {
			return err
		}
	}

	if k.telemetry != nil {
		return k.telemetry.Shutdown(ctx)
	}
	return nil
}

// ReplayEvents fetches events from the store and routes their messages with no_audit=true.
func (k *Kernel) ReplayEvents(ctx context.Context, start, end uint64) error {
	if k.history == nil {
		return errors.New("history store not initialized")
	}

	events, err := k.history.GetRange(start, end)
	if err != nil {
		return err
	}

	// Add special context to skip auditing and prevent recursion
	replayCtx := context.WithValue(ctx, auditContextKey, true)

	for _, ev := range events {
		k.logger.Debug("replaying event", "index", ev.Index, "id", ev.Message.ID)
		k.RouteMessage(replayCtx, ev.Message)
	}

	return nil
}

// RouteMessage handles the delivery of a message to its intended target.
func (k *Kernel) RouteMessage(ctx context.Context, msg api.Message) {
	// Start trace span
	ctx, span := k.tracer.Start(ctx, "kernel.RouteMessage",
		trace.WithAttributes(msg.ToSpanAttributes()...))
	defer span.End()

	k.logger.Debug("kernel routing message", "id", msg.ID, "sender", msg.Sender, "target", msg.Target, "method", msg.Method)
	k.telemetry.RecordMessage(ctx, msg.Sender, msg.Target, msg.Method)

	// Asynchronously log to history (Phase 11: Event Sourcing) unless no_audit is set
	noAudit := false
	if ctx != nil {
		if val, ok := ctx.Value(auditContextKey).(bool); ok {
			noAudit = val
		}
	}
	if !noAudit && msg.Metadata != nil {
		if val, ok := msg.Metadata[auditContextKey].(bool); ok {
			noAudit = val
		} else if sVal, ok := msg.Metadata[auditContextKey].(string); ok {
			noAudit = sVal == "true"
		}
	}

	if k.history != nil && (msg.Type == api.TypeRequest || msg.Type == api.TypeEvent) && !noAudit {

		select {
		case k.eventCh <- msg:
		default:
			// Drop if channel full to avoid blocking routing
		}
	}

	// 1. Resolve Target (Capability resolution) early for IAM
	target := msg.Target
	k.mu.RLock()
	capTarget, isCap := k.capabilityMap[target]
	k.mu.RUnlock()

	if isCap {
		k.logger.Debug("resolved capability to target", "capability", target, "target", capTarget)
		target = capTarget
		msg.Target = target
	}

	// 2. Core Interception (RBAC & Integrated Interceptors)
	if ctx.Value(skipInterceptorsKey) == nil {
		// RBAC Check via built-in IdentityManager
		if !k.insecure {
			actor := msg.Actor
			if actor == "" {
				actor = msg.Sender
			}

			// Security Check: System services are exempt from RBAC
			isSystemService := msg.Sender == "system" || msg.Sender == "kernel" ||
				msg.Sender == "iam" || msg.Sender == "events" ||
				msg.Sender == "widget-manager" || msg.Sender == "command-manager" ||
				msg.Sender == "history"

			if !isSystemService && msg.Type != api.TypeResponse {
				contextID, _ := msg.Metadata["context"].(string)

				if !k.iam.AuthorizeWithContext(actor, msg.Target, msg.Method, contextID) {
					k.logger.Warn("IAM denied routing", "actor", actor, "target", msg.Target, "method", msg.Method, "ctx", contextID)
					k.telemetry.RecordError(ctx, msg.Target, "auth_denied")
					span.SetAttributes(attribute.Bool("alloy.msg.allowed", false))

					// Send error response back if request
					k.deliverToFrontendSync(ctx, msg.Sender, api.Message{
						ID:        msg.ID + "-resp",
						Type:      api.TypeResponse,
						Sender:    "kernel",
						Target:    msg.Sender,
						Payload:   []byte(`{"error":"access denied"}`),
						Timestamp: time.Now().Unix(),
					})
					return
				}
			}
		}

		// External Interceptors
		k.mu.RLock()
		interceptors := k.interceptors
		k.mu.RUnlock()

		for _, interceptor := range interceptors {
			newMsg, cont, err := interceptor.PreRoute(ctx, msg)
			if err != nil {
				k.logger.Error("interceptor error", "error", err)
				return
			}
			if !cont {
				return
			}
			msg = newMsg
		}
	}
	span.SetAttributes(attribute.Bool("alloy.msg.allowed", true))

	// Observability: Emit trace event for TUI/Inspector
	// Prevent loop by ignoring trace events and messages from the event bus itself
	if k.events != nil && msg.ID != "" &&
		!strings.HasPrefix(msg.ID, "trace-") &&
		!strings.HasPrefix(msg.ID, "evt-") &&
		msg.Method != "system:trace" &&
		msg.Sender != "events" &&
		msg.Sender != "logger" &&
		k.events.HasSubscribers("system:trace") {
		tracePayload, _ := json.Marshal(map[string]any{
			"id":     msg.ID,
			"sender": msg.Sender,
			"target": msg.Target,
			"method": msg.Method,
			"time":   time.Now().UnixNano(),
		})
		go k.events.Publish(ctx, "system:trace", "kernel", json.RawMessage(tracePayload))
	}

	// 2. Broadcast and Discovery Handling
	if msg.Target == "" || msg.Target == "*" {
		k.broadcast(ctx, msg)
		return
	}

	// 3. System and Command Management Integration
	if msg.Target == "kernel" || msg.Target == "system" {
		k.handleInternalMessage(ctx, msg)
		return
	}

	if msg.Target == "command-manager" {
		k.handleCommandManagerMessage(ctx, msg)
		// Fall through to deliver to plugin unless handle method consumed it?
		// Actually the command manager IS a registered plugin, so we just deliver.
	}

	// 4. Deliver to Frontend, Plugin, or Lazy Load
	// Note: msg.Target may have been updated by capability resolution
	target = msg.Target

	k.mu.RLock()
	frontendChan, isFrontend := k.frontends[target]
	plugin, isPlugin := k.plugins[target]
	_, hasMetadata := k.metadata[target]
	_, hasLoader := k.loaders[target]
	k.mu.RUnlock()

	if isFrontend {
		select {
		case frontendChan <- msg:
			k.logger.Debug("message delivered to frontend", "target", target)
		default:
			k.logger.Warn("frontend buffer full", "target", target)
		}
		return
	}

	if isPlugin {
		k.deliverToPlugin(ctx, plugin, msg)
		return
	}

	if hasMetadata && hasLoader {
		go k.lazyLoadAndDeliver(ctx, msg)
		return
	}

	k.logger.Warn("message target not found", "target", msg.Target)
	if msg.Type == api.TypeRequest {
		k.deliverToFrontendSync(ctx, msg.Sender, api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    "kernel",
			Target:    msg.Sender,
			Payload:   []byte(`{"error":"target service or capability not found"}`),
			Timestamp: time.Now().Unix(),
		})
	}
}

// HandleMessageSync performs a synchronous message call, resolving capabilities if needed.
// QueryLibrarian performs a semantic search via the Librarian plugin (Phase 11).
func (k *Kernel) QueryLibrarian(ctx context.Context, query string) (string, error) {
	// 1. Prepare search message
	payload, _ := json.Marshal(map[string]any{"query": query, "limit": 3})
	msg := api.Message{
		ID:        "kernel-lib-query-" + fmt.Sprint(time.Now().UnixNano()),
		Type:      api.TypeRequest,
		Sender:    "kernel",
		Target:    "librarian",
		Method:    "librarian:search",
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}

	// 2. Call librarian synchronously
	resp, err := k.HandleMessageSync(ctx, msg)
	if err != nil {
		return "", err
	}

	// 3. Extract content snippets
	var results []struct {
		Document struct {
			Content string `json:"content"`
		} `json:"document"`
	}
	if err := json.Unmarshal(resp.Payload, &results); err != nil {
		return "", err
	}

	var builder strings.Builder
	for _, res := range results {
		if builder.Len() > 0 {
			builder.WriteString("\n---\n")
		}
		builder.WriteString(res.Document.Content)
	}

	return builder.String(), nil
}

func (k *Kernel) HandleMessageSync(ctx context.Context, msg api.Message) (api.Message, error) {
	k.mu.RLock()
	target := msg.Target
	if capTarget, ok := k.capabilityMap[target]; ok {
		target = capTarget
		msg.Target = target
	}
	plugin, ok := k.plugins[target]
	_, hasMeta := k.metadata[target]
	k.mu.RUnlock()

	if !ok && hasMeta {
		if err := k.lazyLoadPlugin(ctx, target); err == nil {
			k.mu.RLock()
			plugin, ok = k.plugins[target]
			k.mu.RUnlock()
		}
	}

	if ok && plugin != nil {
		return plugin.HandleMessage(ctx, msg)
	}

	return api.Message{}, fmt.Errorf("plugin or capability %s not found", msg.Target)
}

// Call is the exported version of HandleMessageSync
func (k *Kernel) Call(ctx context.Context, msg api.Message) (api.Message, error) {
	return k.HandleMessageSync(ctx, msg)
}

func (k *Kernel) deliverToPlugin(ctx context.Context, p api.Plugin, msg api.Message) {
	go func(plugin api.Plugin, m api.Message, parentCtx context.Context) {
		deliveryCtx := context.Background()
		// Propagate context values and spans
		if parentCtx != nil {
			if audit, ok := parentCtx.Value(auditContextKey).(bool); ok {
				deliveryCtx = context.WithValue(deliveryCtx, auditContextKey, audit)
			}
			if skip, ok := parentCtx.Value(skipInterceptorsKey).(bool); ok {
				deliveryCtx = context.WithValue(deliveryCtx, skipInterceptorsKey, skip)
			}
			if span := trace.SpanFromContext(parentCtx); span.SpanContext().IsValid() {
				deliveryCtx = trace.ContextWithSpan(deliveryCtx, span)
			}
		}

		childCtx, childSpan := k.tracer.Start(deliveryCtx, "plugin.HandleMessage",
			trace.WithAttributes(attribute.String("alloy.plugin.id", plugin.ID())))
		defer childSpan.End()

		resp, err := plugin.HandleMessage(childCtx, m)
		if err != nil {
			k.logger.Error("plugin error", "plugin_id", m.Target, "error", err)
			childSpan.RecordError(err)
			return
		}

		if resp.ID != "" && resp.Target != "" {
			k.RouteMessage(childCtx, resp)
		}
	}(p, msg, ctx)
}

func (k *Kernel) broadcast(ctx context.Context, msg api.Message) {
	k.mu.RLock()
	frontends := make(map[string]chan<- api.Message, len(k.frontends))
	for id, ch := range k.frontends {
		frontends[id] = ch
	}
	k.mu.RUnlock()

	for _, ch := range frontends {
		select {
		case ch <- msg:
		case <-ctx.Done():
			return
		case <-k.stopCh:
			return
		default:
		}
	}
}

func (k *Kernel) lazyLoadAndDeliver(ctx context.Context, msg api.Message) {
	k.logger.Debug("triggering lazy-load for plugin", "id", msg.Target)
	if err := k.lazyLoadPlugin(ctx, msg.Target); err == nil {
		k.mu.RLock()
		plugin, ok := k.plugins[msg.Target]
		k.mu.RUnlock()
		if ok {
			k.deliverToPlugin(ctx, plugin, msg)
		}
	} else {
		k.logger.Error("lazy-load failed", "id", msg.Target, "error", err)
		if msg.Type == api.TypeRequest {
			k.deliverToFrontendSync(ctx, msg.Sender, api.Message{
				ID:        msg.ID + "-resp",
				Type:      api.TypeResponse,
				Sender:    "kernel",
				Target:    msg.Sender,
				Payload:   []byte(fmt.Sprintf(`{"error":"failed to load plugin: %s"}`, err.Error())),
				Timestamp: time.Now().Unix(),
			})
		}
	}
}

func (k *Kernel) lazyLoadPlugin(ctx context.Context, pluginID string) error {
	k.mu.Lock()
	if _, ok := k.plugins[pluginID]; ok {
		k.mu.Unlock()
		return nil
	}
	if ch, inProgress := k.loading[pluginID]; inProgress {
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

	defer func() {
		k.mu.Lock()
		delete(k.loading, pluginID)
		close(ch)
		k.mu.Unlock()
	}()

	k.mu.RLock()
	loader, hasLoader := k.loaders[pluginID]
	k.mu.RUnlock()

	if !hasLoader {
		return errors.New("loader not found")
	}

	p, err := loader.LoadPlugin(ctx, pluginID)
	if err != nil {
		return err
	}

	k.RegisterPlugin(p)
	return nil
}

// RegisterPlugin attaches an active plugin to the kernel. (Phase 10)
func (k *Kernel) RegisterPlugin(p api.Plugin) {
	id := p.ID()
	k.mu.Lock()

	// Skip re-registration for core plugins
	if existing, ok := k.plugins[id]; ok && existing != nil {
		switch id {
		case "events", "command-manager", "wasm-manager", "iam", "health", "logger", "kv", "storage", "network", "cache", "doc", "widget-manager":
			k.mu.Unlock()
			return
		}
	} else {
		k.telemetry.PluginCountChange(context.Background(), 1)
	}

	k.plugins[id] = p
	caps := p.Capabilities()
	var intents []string
	for _, cap := range caps {
		intents = append(intents, cap.Intents...)
	}

	// Try to get background status/intents from p if it's a witPluginWrapper or implements background interface (Phase 10)
	background := false
	sidecar := false
	if wp, ok := p.(*witPluginWrapper); ok {
		background = wp.background
		sidecar = wp.sidecar
		// Also try to get enriched metadata from Wasm Manager if available
		if meta, ok := k.wasmManager.GetPluginMetadata(id); ok {
			if meta.Background {
				background = true
			}
			if meta.Sidecar {
				sidecar = true
			}
			if len(meta.Intents) > 0 {
				intents = meta.Intents
			}
		}
	} else if ba, ok := p.(interface {
		IsBackground() bool
	}); ok {
		background = ba.IsBackground()
	}

	if sidecar && k.iam != nil {
		k.iam.AssignRole(id, "admin")
	}

	k.metadata[id] = api.PluginMetadata{
		ID:           id,
		Capabilities: caps,
		Intents:      intents,
		Background:   background,
		Sidecar:      sidecar,
	}
	k.mu.Unlock() // Unlock before intent registration if it needs k.mu (unlikely but safe)

	// Metadata registered in the intent broker (Phase 10)
	k.intents.Register(id, intents)

	k.mu.Lock() // Re-lock for capability map update
	// Update capability map
	for _, cap := range caps {
		if cap.Method != "" {
			k.capabilityMap[cap.Method] = id
		}
	}
	// Automatically register if it implements Interceptor
	if i, ok := p.(api.Interceptor); ok {
		k.interceptors = append(k.interceptors, i)
		k.logger.Info("interceptor registered", "plugin_id", p.ID())
	}
	k.mu.Unlock()

	k.logger.Info("plugin registered", "plugin_id", id, "background", background)

	k.logger.Info("plugin registered", "plugin_id", id)

	// Inject router if accepted
	type routerAcceptor interface {
		SetRouter(func(context.Context, api.Message))
	}
	if ra, ok := p.(routerAcceptor); ok {
		ra.SetRouter(k.RouteMessage)
	}

	// Emit registration event
	if k.events != nil {
		go func() {
			caps := p.Capabilities()
			capsData, _ := json.Marshal(caps)
			k.events.Publish(context.Background(), "component:registered", "kernel",
				[]byte(`{"id":"`+id+`","type":"plugin","capabilities":`+string(capsData)+`}`))
		}()
	}
}

// DispatchIntent dispatches an intent to be routed by the kernel (Phase 10)
func (k *Kernel) DispatchIntent(ctx context.Context, intent api.Intent) error {
	return k.intents.Dispatch(ctx, intent)
}

// RegisterMetadata describes a plugin to the kernel without loading it yet. (Phase 10)
func (k *Kernel) RegisterMetadata(info api.PluginMetadata, loader api.PluginLoader) {
	k.mu.Lock()
	k.metadata[info.ID] = info
	k.loaders[info.ID] = loader
	for _, cap := range info.Capabilities {
		if cap.Method != "" {
			k.capabilityMap[cap.Method] = info.ID
		}
	}
	k.mu.Unlock()

	// Register intents in broker for lazy-loading lookup (optional if broker only handles loaded plugins, but good for discovery)
	k.intents.Register(info.ID, info.Intents)

	k.logger.Info("plugin metadata registered", "plugin_id", info.ID)

	go func() {
		metaData, _ := json.Marshal(api.Registration{
			ID:           info.ID,
			Type:         "plugin-meta",
			Capabilities: info.Capabilities,
			Intents:      info.Intents,
			Background:   info.Background,
		})
		k.events.Publish(context.Background(), "component:registered", "kernel", metaData)
	}()
}

// RegisterFrontend registers a frontend's response channel.
func (k *Kernel) RegisterFrontend(id string, ch chan<- api.Message) {
	k.mu.Lock()
	k.frontends[id] = ch
	k.mu.Unlock()

	k.logger.Info("frontend registered", "frontend_id", id)

	go k.events.Publish(context.Background(), "component:registered", "kernel",
		[]byte(`{"id":"`+id+`","type":"frontend"}`))

	// Push current system state to the new frontend
	go func() {
		time.Sleep(100 * time.Millisecond) // Give the frontend a moment to start its read loop

		// 2. Push all registered widgets
		widgets := k.ListWidgets()
		for _, w := range widgets {
			wData, _ := json.Marshal(w)
			k.deliverToFrontendSync(context.Background(), id, api.Message{
				ID:      fmt.Sprintf("init-widget-reg-%s", w.ID),
				Type:    api.TypeEvent,
				Sender:  "kernel",
				Target:  id,
				Method:  "dashboard:widget-registered",
				Payload: wData,
			})
		}
	}()
}

func (k *Kernel) UnregisterFrontend(id string) {
	k.mu.Lock()
	delete(k.frontends, id)
	k.mu.Unlock()
}

func (k *Kernel) deliverToFrontendSync(ctx context.Context, id string, msg api.Message) {
	k.mu.RLock()
	ch, ok := k.frontends[id]
	k.mu.RUnlock()

	if ok {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (k *Kernel) listRegistrations() []api.Registration {
	k.mu.RLock()
	defer k.mu.RUnlock()
	var regs []api.Registration
	for id, meta := range k.metadata {
		regs = append(regs, api.Registration{
			ID:           id,
			Type:         "plugin",
			Status:       "active",
			Capabilities: meta.Capabilities,
		})
	}
	// Add frontends
	for id := range k.frontends {
		regs = append(regs, api.Registration{
			ID:   id,
			Type: "frontend",
		})
	}
	return regs
}

func (k *Kernel) handleInternalMessage(ctx context.Context, msg api.Message) {
	if msg.Type != api.TypeRequest {
		return
	}

	switch msg.Method {
	case "ping":
		k.deliverToFrontendSync(ctx, msg.Sender, api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    "kernel",
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"pong"}`),
			Timestamp: time.Now().Unix(),
		})
	case "intent:dispatch":
		var intent api.Intent
		if err := json.Unmarshal(msg.Payload, &intent); err == nil {
			if intent.ID == "" {
				intent.ID = msg.ID
			}
			if intent.Sender == "" {
				intent.Sender = msg.Sender
			}
			k.intents.Dispatch(ctx, intent)
		}
	case "discovery:list", "list":
		msg.Target = "command-manager"
		k.RouteMessage(ctx, msg)
	case "stop":
		k.Shutdown(ctx)
	}
}

func (k *Kernel) handleCommandManagerMessage(ctx context.Context, msg api.Message) {
	if msg.Method == "register" {
		var reg api.Registration
		if err := json.Unmarshal(msg.Payload, &reg); err == nil {
			id := reg.ID
			if id == "" {
				id = msg.Sender
			}
			k.mu.Lock()
			md := k.metadata[id]
			md.ID = id
			md.Capabilities = reg.Capabilities
			k.metadata[id] = md

			for _, cap := range reg.Capabilities {
				if cap.Method != "" {
					k.capabilityMap[cap.Method] = id
				}
			}
			k.mu.Unlock()
		}
	} else if msg.Method == "register-capability" {
		var cap api.Capability
		if err := json.Unmarshal(msg.Payload, &cap); err == nil {
			if cap.Method != "" {
				id := msg.Sender
				k.mu.Lock()
				k.capabilityMap[cap.Method] = id

				// Update metadata as well
				md := k.metadata[id]
				md.ID = id
				found := false
				for i, existing := range md.Capabilities {
					if existing.Method == cap.Method {
						md.Capabilities[i] = cap
						found = true
						break
					}
				}
				if !found {
					md.Capabilities = append(md.Capabilities, cap)
				}
				k.metadata[id] = md
				k.mu.Unlock()
			}
		}
	}
}

// Provisioning

type PluginDef struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"` // "wasm" (native is now built-in or handled differently)
	Path         string             `json:"path,omitempty"`
	MaxMemoryMB  uint32             `json:"max_memory_mb,omitempty"`
	MsgPerSecond int                `json:"msg_per_second,omitempty"`
	LoadTime     api.PluginLoadTime `json:"load_time,omitempty"`
	Capabilities []api.Capability   `json:"capabilities,omitempty"`
	Background   bool               `json:"background,omitempty"` // Phase 10
	Sidecar      bool               `json:"sidecar,omitempty"`    // Phase 10
}

func (k *Kernel) Provision(plugins []PluginDef) error {
	for _, p := range plugins {
		if p.Type == "wasm" {
			if p.LoadTime == api.LoadTimeLazy {
				k.RegisterMetadata(api.PluginMetadata{
					ID:           p.ID,
					LoadTime:     api.LoadTimeLazy,
					Capabilities: p.Capabilities,
					Background:   p.Background,
					Sidecar:      p.Sidecar,
				}, &wasmLoader{
					k:            k,
					pluginID:     p.ID,
					path:         p.Path,
					logger:       k.logger,
					maxMemoryMB:  p.MaxMemoryMB,
					msgPerSecond: p.MsgPerSecond,
					capabilities: p.Capabilities,
					background:   p.Background,
					sidecar:      p.Sidecar,
				})
			} else {
				// Immediate load
				wasmBytes, err := os.ReadFile(p.Path)
				if err != nil {
					k.logger.Error("failed to read plugin WASM for boot-load", "id", p.ID, "path", p.Path, "error", err)
					continue
				}
				if err := k.RegisterWASMPluginAtScale(p.ID, wasmBytes, p.MaxMemoryMB, p.MsgPerSecond, p.Capabilities, p.Background, p.Sidecar); err != nil {
					k.logger.Error("failed to register boot-loaded plugin", "id", p.ID, "error", err)
				}
			}
		} else if p.Type == "native" {
			// Check if it's already a built-in core service
			if _, ok := k.plugins[p.ID]; ok {
				k.logger.Debug("plugin already integrated as core service", "id", p.ID)
				continue
			}
			k.logger.Warn("native plugin type is deprecated; core services are now built-in", "id", p.ID)
		}
	}
	return nil
}

// wasmLoader implements the api.PluginLoader interface for lazy-loading WASM plugins.
type wasmLoader struct {
	k            *Kernel
	pluginID     string
	path         string
	logger       *slog.Logger
	maxMemoryMB  uint32
	msgPerSecond int
	capabilities []api.Capability
	background   bool
	sidecar      bool
}

func (l *wasmLoader) LoadPlugin(ctx context.Context, id string) (api.Plugin, error) {
	l.logger.Info("lazy-loading plugin", "id", id, "path", l.path)

	wasmBytes, err := os.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read lazy-loaded WASM: %w", err)
	}

	// Defaults
	if l.maxMemoryMB == 0 {
		l.maxMemoryMB = 128
	}
	if l.msgPerSecond == 0 {
		l.msgPerSecond = 1000
	}

	if err := l.k.RegisterWASMPluginAtScale(id, wasmBytes, l.maxMemoryMB, l.msgPerSecond, l.capabilities, l.background, l.sidecar); err != nil {
		return nil, fmt.Errorf("failed to register lazy-loaded WASM: %w", err)
	}

	p, ok := l.k.GetPlugin(id)
	if !ok {
		return nil, fmt.Errorf("plugin %s registered but could not be retrieved", id)
	}

	return p, nil
}

// WIT/WASM helpers

// RegisterWASMPluginAtScale registers a WASM plugin with the kernel. (Phase 10)
func (k *Kernel) RegisterWASMPluginAtScale(pluginID string, wasmBytes []byte, maxMemoryMB uint32, msgPerSec int, caps []api.Capability, background bool, sidecar bool) error {
	plugin := &witPluginWrapper{
		id:         pluginID,
		manager:    k.wasmManager,
		caps:       caps,
		sidecar:    sidecar,
		background: background,
	}

	k.RegisterPlugin(plugin)
	return k.wasmManager.LoadPlugin(context.Background(), pluginID, wasmBytes, maxMemoryMB, msgPerSec, caps, background)
}

// RegisterWASMPlugin registers a WASM plugin with default limits. (Phase 10)
func (k *Kernel) RegisterWASMPlugin(pluginID string, wasmBytes []byte, caps []api.Capability) error {
	return k.RegisterWASMPluginAtScale(pluginID, wasmBytes, 128, 1000, caps, false, false)
}

// ResolvePluginPath attempts to find the WASM file relative to several well-known locations.
func ResolvePluginPath(manifestPath, pluginPath string) string {
	if filepath.IsAbs(pluginPath) {
		if _, err := os.Stat(pluginPath); err == nil {
			return pluginPath
		}
	}

	// 1. Try relative to the manifest file itself
	relToManifest := filepath.Join(filepath.Dir(manifestPath), pluginPath)
	if _, err := os.Stat(relToManifest); err == nil {
		return relToManifest
	}

	// 2. Try the official FHS location relative to the binary
	if exe, err := os.Executable(); err == nil {
		fhsPath := filepath.Join(filepath.Dir(exe), "..", "lib", "alloy", "plugins", pluginPath)
		if _, err := os.Stat(fhsPath); err == nil {
			return fhsPath
		}
	}

	// 3. Try common dev paths (relative to CWD)
	cwd, _ := os.Getwd()
	devPaths := []string{
		filepath.Join(cwd, "build", "dist", "usr", "lib", "alloy", "plugins", pluginPath),
		filepath.Join(cwd, "build", "plugins", pluginPath),
		filepath.Join(cwd, "build", "bin", pluginPath),
	}
	for _, p := range devPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// 4. Try system-wide location
	sysPath := filepath.Join("/usr/lib/alloy/plugins", pluginPath)
	if _, err := os.Stat(sysPath); err == nil {
		return sysPath
	}

	return ""
}

func (k *Kernel) GetPluginMetadata() map[string]api.PluginMetadata {
	k.mu.RLock()
	defer k.mu.RUnlock()
	meta := make(map[string]api.PluginMetadata, len(k.metadata))
	for id, m := range k.metadata {
		meta[id] = m
	}
	return meta
}

func (k *Kernel) GetPlugin(id string) (api.Plugin, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	p, ok := k.plugins[id]
	return p, ok
}

func (k *Kernel) RegisterInterceptor(i api.Interceptor) {
	k.mu.Lock()
	k.interceptors = append(k.interceptors, i)
	k.mu.Unlock()
}

func (k *Kernel) ListWidgets() []api.Widget { return k.widgets.ListWidgets() }

// Internal helper for wasm manager
type witPluginWrapper struct {
	id         string
	manager    *wasm.Manager
	caps       []api.Capability
	sidecar    bool
	background bool
}

func (p *witPluginWrapper) ID() string                     { return p.id }
func (p *witPluginWrapper) IsBackground() bool             { return p.background }
func (p *witPluginWrapper) Capabilities() []api.Capability { return p.caps }
func (p *witPluginWrapper) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	if msg.Type == api.TypeRequest {
		err := p.manager.RouteMessage(ctx, p.id, msg)
		if err != nil {
			return api.Message{}, err
		}
		return p.manager.GetResponse(ctx, p.id, msg.ID)
	}
	return api.Message{}, p.manager.RouteMessage(ctx, p.id, msg)
}
func (p *witPluginWrapper) Shutdown(ctx context.Context) error {
	return p.manager.UnloadPlugin(ctx, p.id)
}

type wasmManagerPlugin struct {
	kernel *Kernel
}

func (w *wasmManagerPlugin) ID() string { return "wasm-manager" }
func (w *wasmManagerPlugin) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "load", Description: "Load a WASM plugin"},
		{Method: "unload", Description: "Unload a WASM plugin"},
	}
}
func (w *wasmManagerPlugin) Shutdown(ctx context.Context) error { return nil }
func (w *wasmManagerPlugin) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "load":
		var req struct {
			ID           string           `json:"id"`
			Path         string           `json:"path"`
			MaxMemoryMB  uint32           `json:"max_memory_mb"`
			Capabilities []api.Capability `json:"capabilities"`
			Background   bool             `json:"background"` // Phase 10
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}
		wasmBytes, err := os.ReadFile(req.Path)
		if err != nil {
			return api.Message{}, err
		}
		err = w.kernel.RegisterWASMPluginAtScale(req.ID, wasmBytes, req.MaxMemoryMB, 1000, req.Capabilities, req.Background, false)
		if err != nil {
			return api.Message{}, err
		}
		return api.Message{ID: msg.ID + "-resp", Type: api.TypeResponse, Sender: w.ID(), Target: msg.Sender, Payload: []byte(`{"status":"loaded"}`)}, nil
	case "unload":
		var req struct{ ID string }
		json.Unmarshal(msg.Payload, &req)
		w.kernel.wasmManager.UnloadPlugin(ctx, req.ID)
		return api.Message{ID: msg.ID + "-resp", Type: api.TypeResponse, Sender: w.ID(), Target: msg.Sender, Payload: []byte(`{"status":"unloaded"}`)}, nil
	}
	return api.Message{}, nil
}
