package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

// Bridge provides a Go-native interface for kernel components to use
// core services with the same semantics as WASM plugins.
// This implements a "Kernel-Guest" pattern.
type Bridge struct {
	k  *Kernel
	id string // The identity of the component using the bridge
}

// NewBridge creates a new bridge for a specific kernel component.
func (k *Kernel) NewBridge(id string) *Bridge {
	return &Bridge{k: k, id: id}
}

// KV Utils

func (b *Bridge) KVSet(key string, val []byte) bool {
	return b.k.storage.Set(b.id, key, val) == nil
}

func (b *Bridge) KVGet(key string) ([]byte, bool) {
	val, err := b.k.storage.Get(b.id, key)
	if err != nil || val == nil {
		return nil, false
	}
	return val, true
}

func (b *Bridge) KVDelete(key string) bool {
	return b.k.storage.Delete(b.id, key) == nil
}

func (b *Bridge) KVList(prefix string) []string {
	keys, _ := b.k.storage.List(b.id, prefix)
	return keys
}

// Event Utils

func (b *Bridge) Publish(topic string, data any) {
	payload, _ := json.Marshal(map[string]any{
		"topic": topic,
		"data":  data,
	})
	b.k.RouteMessage(context.Background(), api.Message{
		ID:      fmt.Sprintf("bridge-pub-%d", time.Now().UnixNano()),
		Type:    api.TypeEvent,
		Sender:  b.id,
		Target:  "events",
		Method:  "publish",
		Payload: payload,
	})
}

func (b *Bridge) Subscribe(topic string) {
	payload, _ := json.Marshal(map[string]string{
		"topic": topic,
	})
	b.k.RouteMessage(context.Background(), api.Message{
		ID:      fmt.Sprintf("bridge-sub-%d", time.Now().UnixNano()),
		Type:    api.TypeRequest,
		Sender:  b.id,
		Target:  "events",
		Method:  "subscribe",
		Payload: payload,
	})
}

// Routing Utils

func (b *Bridge) Call(target, method string, payload any) (api.Message, error) {
	data, _ := json.Marshal(payload)
	msg := api.Message{
		ID:      fmt.Sprintf("bridge-call-%d", time.Now().UnixNano()),
		Type:    api.TypeRequest,
		Sender:  b.id,
		Target:  target,
		Method:  method,
		Payload: data,
	}
	return b.k.HandleMessageSync(context.Background(), msg)
}

// Log logs a message through the kernel logger.
func (b *Bridge) Log(level, msg string) {
	b.k.logger.Info("bridge_log", "id", b.id, "level", level, "msg", msg)
}
