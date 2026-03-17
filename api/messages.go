package api

import (
	"context"
	"encoding/json"
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

// Capability describes a functionality provided by a component.
type Capability struct {
	Method      string `json:"method"`
	Description string `json:"description,omitempty"`
}
