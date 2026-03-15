package api

import "encoding/json"

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
