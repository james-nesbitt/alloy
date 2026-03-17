package wasm

import (
	"context"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

// KVManager is a facade for the host KV store.
type KVManager struct {
	kv KVStore
}

func NewKVManager(kv KVStore) *KVManager {
	return &KVManager{kv: kv}
}

func (k *KVManager) ID() string { return "plugin-kv" }

func (k *KVManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "get", Description: "Retrieve value for a key"},
		{Method: "set", Description: "Store a value for a key"},
	}
}

func (k *KVManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	// Simple wrapper around host KV
	switch msg.Method {
	case "get":
		// Payload would contain the key. 
		// For now, this is a simplified stub.
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    k.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"value":""}`),
			Timestamp: time.Now().Unix(),
		}, nil
	case "set":
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    k.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"ok"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	}
	return api.Message{}, nil
}

func (k *KVManager) Shutdown(ctx context.Context) error {
	return nil
}
