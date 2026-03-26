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
}

// Workspace represents a project or team-level context.
type Workspace struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Path     string            `json:"path"`
	TeamID   string            `json:"team_id,omitempty"`
	Layout   string            `json:"layout,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Presence represents a user's status in the system.
type Presence struct {
	User      string `json:"user"`
	Status    string `json:"status"`
	LastSeen  int64  `json:"last_seen"`
	Client    string `json:"client"`
	ProjectID string `json:"project_id,omitempty"`
}

// Registration defines a component's presence in the system.
type Registration struct {
	ID           string       `json:"id"`
	Type         string       `json:"type"`
	Status       string       `json:"status,omitempty"`
	Capabilities []Capability `json:"capabilities,omitempty"`
}

// Widget represents a dynamic dashboard tile.
type Widget struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	ContentType       string `json:"content_type"` // "markdown", "json", "ascii-art"
	Content           []byte `json:"content"`
	RefreshIntervalMs uint32 `json:"refresh_interval_ms"`
}

// WorkspaceConfig defines the visual and operational layout for a project.
type WorkspaceConfig struct {
	DefaultMode string `json:"default_mode"`
	Layout      []struct {
		Type     string  `json:"type"` // "dashboard", "chat", "editor", "status"
		WidthPct float64 `json:"width_pct"`
	} `json:"layout"`
}

// Project represents a logical unit of work.
type Project struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Layout      WorkspaceConfig `json:"layout,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
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
}

// Buffer reflects a handle to shared memory or file-backed storage
type Buffer struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Content      []byte `json:"content"`
	Size         int    `json:"size"`
	LastModified int64  `json:"last_modified,omitempty"`
}
