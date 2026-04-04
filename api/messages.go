package api

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// MessageType defines the type of message being sent.
type MessageType string

const (
	TypeRequest  MessageType = "request"
	TypeResponse MessageType = "response"
	TypeEvent    MessageType = "event"
)

// Message is the standard unit of communication in Alloy.
type Message struct {
	ID        string          `json:"id"`
	Type      MessageType     `json:"type"`
	Sender    string          `json:"sender"`
	Target    string          `json:"target,omitempty"`
	Method    string          `json:"method"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp int64           `json:"timestamp"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
	Actor     string          `json:"actor,omitempty"`      // The identity performing the action (e.g., user email)
	SessionID string          `json:"session_id,omitempty"` // The unique session for the actor
}

// PluginLoadTime defines when a plugin should be loaded.
type PluginLoadTime string

const (
	LoadTimeBoot PluginLoadTime = "boot"
	LoadTimeLazy PluginLoadTime = "lazy"
)

// PluginMetadata describes a plugin's identity and capabilities before it is fully loaded.
type PluginMetadata struct {
	ID           string         `json:"id"`
	Capabilities []Capability   `json:"capabilities"`
	LoadTime     PluginLoadTime `json:"load_time"`
	Intents      []string       `json:"intents,omitempty"`    // Aggregated list of intents this plugin can handle (Phase 10)
	Background   bool           `json:"background,omitempty"` // Whether this plugin runs as a background actor (Phase 10)
	Sidecar      bool           `json:"sidecar,omitempty"`    // Whether this plugin is a global sidecar (Phase 10)
	Headless     bool           `json:"headless,omitempty"`   // Whether this plugin/client is headless (Phase 12)
}

// PluginLoader is an interface for components that can load a plugin on demand.
type PluginLoader interface {
	LoadPlugin(ctx context.Context, id string) (Plugin, error)
}

// Plugin defines the interface for components that can be registered with the kernel.
type Plugin interface {
	ID() string
	Capabilities() []Capability
	HandleMessage(ctx context.Context, msg Message) (Message, error)
	Shutdown(ctx context.Context) error
}

// ReadinessProvider is an optional interface for plugins that need time to
// initialize after loading but before receiving their first message.
type ReadinessProvider interface {
	Ready(ctx context.Context) error
}

func (m Message) ToSpanAttributes() []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("alloy.msg.id", m.ID),
		attribute.String("alloy.msg.sender", m.Sender),
		attribute.String("alloy.msg.target", m.Target),
		attribute.String("alloy.msg.method", m.Method),
		attribute.String("alloy.msg.type", string(m.Type)),
	}
}

// SpanContext returns an OpenTelemetry SpanContext if trace metadata is present in the message.
func (m Message) SpanContext() (trace.SpanContext, bool) {
	if m.Metadata == nil {
		return trace.SpanContext{}, false
	}

	traceIDStr, ok1 := m.Metadata["trace_id"].(string)
	spanIDStr, ok2 := m.Metadata["span_id"].(string)
	if !ok1 || !ok2 {
		return trace.SpanContext{}, false
	}

	traceID, err := trace.TraceIDFromHex(traceIDStr)
	if err != nil {
		return trace.SpanContext{}, false
	}
	spanID, err := trace.SpanIDFromHex(spanIDStr)
	if err != nil {
		return trace.SpanContext{}, false
	}

	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
		Remote:  true,
	}), true
}

// InjectSpanContext adds OpenTelemetry trace metadata to the message.
func (m *Message) InjectSpanContext(sc trace.SpanContext) {
	if m.Metadata == nil {
		m.Metadata = make(map[string]any)
	}
	m.Metadata["trace_id"] = sc.TraceID().String()
	m.Metadata["span_id"] = sc.SpanID().String()
}

// Interceptor is an optional interface that plugins can implement to intercept
// and potentially filter or modify messages before they are routed by the kernel.
type Interceptor interface {
	PreRoute(ctx context.Context, msg Message) (Message, bool, error)
}

// Capability describes a functionality provided by a component.
type Capability struct {
	Method      string            `json:"method,omitempty"`
	Description string            `json:"description,omitempty"`
	Shortcut    string            `json:"shortcut,omitempty"`    // Keyboard shortcut/mnemonic (e.g., "b l")
	Annotations map[string]string `json:"annotations,omitempty"` // Additional metadata (e.g., {"group": "buffers"})
	Intents     []string          `json:"intents,omitempty"`     // Intents satisfied by this method (Phase 10)
}

// Intent structure for goal-oriented routing (Phase 10)
type Intent struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"` // e.g., "intent:save"
	Sender    string          `json:"sender"`
	Target    string          `json:"target,omitempty"` // Specific target if known (Phase 12)
	Payload   json.RawMessage `json:"payload,omitempty"`
	ContextID string          `json:"context_id,omitempty"`
}

// Proposal represents a proactive suggestion from an actor (Phase 12)
type Proposal struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`       // e.g., "Fix Bug"
	Description string          `json:"description"` // e.g., "I noticed a security flaw, should I run an audit?"
	Action      string          `json:"action"`      // The intent name to trigger if accepted
	Payload     json.RawMessage `json:"payload,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
	Confidence  float32         `json:"confidence,omitempty"`
}

// Registration defines a component's presence in the system.
type Registration struct {
	ID           string       `json:"id"`
	Type         string       `json:"type"`
	Status       string       `json:"status,omitempty"`
	Capabilities []Capability `json:"capabilities,omitempty"`
	Background   bool         `json:"background,omitempty"` // Phase 10
	Sidecar      bool         `json:"sidecar,omitempty"`    // Phase 10
	Intents      []string     `json:"intents,omitempty"`    // Phase 10
	Headless     bool         `json:"headless,omitempty"`   // Phase 12
}

// WidgetUpdate represents a content refresh for a specific widget.
type WidgetUpdate struct {
	ID      string `json:"id"`
	Content []byte `json:"content"`
}

// Widget represents a dynamic dashboard tile.
type Widget struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	ContentType       string `json:"content_type"` // "markdown", "json", "ascii-art"
	Content           []byte `json:"content"`
	RefreshIntervalMs uint32 `json:"refresh_interval_ms"`
}

// MmapRegistry defines the interface for mapping buffers in WASM.
type MmapRegistry interface {
	GetBuffer(id string) (SharedBuffer, bool)
	CreateBuffer(id, name string, initialSize int) (SharedBuffer, error)
}

// SharedBuffer represents a host-side shared memory region.
type SharedBuffer interface {
	GetID() string
	GetName() string
	GetData() []byte
	GetSize() int
	Lock()
	Unlock()
	GetVersion() int
	GetLastModified() int64
	Resize(newSize int) error
	ApplyChange(change BufferChange) error
	OnUpdate(callback func(id string, offset int, length int))
	VisualIntent(intent VisualIntent) error
}

// BufferChange represents a mutation to a buffer.
type BufferChange struct {
	Offset    int    `json:"offset"`
	Data      []byte `json:"data"`
	Version   int    `json:"version"`
	Actor     string `json:"actor"`
	Timestamp int64  `json:"timestamp"`
}

// Buffer reflects a handle to shared memory or file-backed storage
type Buffer struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Content      []byte `json:"content"`
	Size         int    `json:"size"`
	LastModified int64  `json:"last_modified,omitempty"`
}

// VisualIntent represents a virtual cursor or highlight (Phase 12)
type VisualIntent struct {
	BufferID string `json:"buffer_id"`
	ActorID  string `json:"actor_id"`
	Type     string `json:"type"` // "cursor", "highlight"
	Offset   int    `json:"offset"`
	Length   int    `json:"length,omitempty"`
	Color    string `json:"color,omitempty"`
	Label    string `json:"label,omitempty"`
}

// Attestation represents a cryptographic proof of identity or role (Phase 12)
type Attestation struct {
	ID        string `json:"id"`
	Actor     string `json:"actor"`
	Role      string `json:"role,omitempty"`
	Target    string `json:"target,omitempty"` // Optional target this attestation is for
	Timestamp int64  `json:"timestamp"`
	PublicKey []byte `json:"public_key,omitempty"`
	Signature []byte `json:"signature"`
	Hardware  string `json:"hardware,omitempty"`
}

// Delegation represents a multi-step task assigned to an actor (Phase 12)
type Delegation struct {
	ID          string          `json:"id"`
	ParentID    string          `json:"parent_id,omitempty"`
	Owner       string          `json:"owner"`                // Assigner
	Assignee    string          `json:"assignee"`             // Agent
	Status      string          `json:"status"`               // "pending", "in_progress", "complete", "failed"
	Task        string          `json:"task"`                 // Goal description
	Payload     json.RawMessage `json:"payload,omitempty"`
	Attestation *Attestation    `json:"attestation,omitempty"`
	Chain       []string        `json:"chain,omitempty"`      // IDs of sub-task delegations
}
