//go:build tinygo || wasip1 || wasm
package wasm

import (
	"encoding/json"
	"fmt"
	"time"
)

// Events provides a high-level API for sending and receiving events.
type Events struct {
	pluginID string
}

// NewEvents creates a new events provider for the given plugin.
func NewEvents(pluginID string) *Events {
	return &Events{pluginID: pluginID}
}

// Emit broadcasts an event to the system.
func (e *Events) Emit(topic string, payload interface{}) bool {
	data, err := json.Marshal(payload)
	if err != nil {
		Log(fmt.Sprintf("Failed to marshal event payload: %v", err))
		return false
	}
	
	msg := Message{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      "event",
		Sender:    e.pluginID,
		Method:    topic,
		Payload:   data,
		Timestamp: time.Now().Unix(),
	}
	
	return RouteMessage(msg)
}

// Subscribe sends a subscription request to the plugin-events manager.
func (e *Events) Subscribe(topic string) bool {
	payload, _ := json.Marshal(map[string]string{"topic": topic})
	msg := Message{
		ID:        fmt.Sprintf("sub-%d", time.Now().UnixNano()),
		Type:      "request",
		Sender:    e.pluginID,
		Target:    "plugin-events",
		Method:    "subscribe",
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}
	return RouteMessage(msg)
}

// Unsubscribe removes a subscription from the plugin-events manager.
func (e *Events) Unsubscribe(topic string) bool {
	payload, _ := json.Marshal(map[string]string{"topic": topic})
	msg := Message{
		ID:        fmt.Sprintf("unsub-%d", time.Now().UnixNano()),
		Type:      "request",
		Sender:    e.pluginID,
		Target:    "plugin-events",
		Method:    "unsubscribe",
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}
	return RouteMessage(msg)
}
