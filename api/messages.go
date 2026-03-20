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
}

// Plugin defines the interface for components that can be registered with the kernel.
type Plugin interface {
	ID() string
	Capabilities() []Capability
	HandleMessage(ctx context.Context, msg Message) (Message, error)
	Shutdown(ctx context.Context) error
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
