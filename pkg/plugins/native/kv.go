package native

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
)

// KVManager is a facade for the host KV store.
type KVManager struct {
	kv storage.StateStore
}

func NewKVManager(kv storage.StateStore) *KVManager {
	return &KVManager{kv: kv}
}

func (k *KVManager) ID() string { return "plugin-kv" }

func (k *KVManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "get", Description: "Retrieve value for a key"},
		{Method: "set", Description: "Store a value for a key"},
		{Method: "list", Description: "List keys with a prefix"},
	}
}

type KVRequest struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Prefix string `json:"prefix,omitempty"`
}

func (k *KVManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	var req KVRequest
	_ = json.Unmarshal(msg.Payload, &req)

	switch msg.Method {
	case "list":
		keys, err := k.kv.List(msg.Sender, req.Prefix)
		if err != nil {
			return api.Message{}, err
		}
		resp := map[string][]string{
			"keys": keys,
		}
		respPayload, _ := json.Marshal(resp)
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    k.ID(),
			Target:    msg.Sender,
			Payload:   respPayload,
			Timestamp: time.Now().Unix(),
		}, nil
	case "get":
		val, err := k.kv.Get(msg.Sender, req.Key)
		if err != nil {
			return api.Message{}, err
		}
		if val == nil {
			val = []byte("")
		}
		resp := map[string]string{
			"key":   req.Key,
			"value": string(val),
		}
		respPayload, _ := json.Marshal(resp)
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    k.ID(),
			Target:    msg.Sender,
			Payload:   respPayload,
			Timestamp: time.Now().Unix(),
		}, nil
	case "set":
		err := k.kv.Set(msg.Sender, req.Key, []byte(req.Value))
		if err != nil {
			return api.Message{}, err
		}
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    k.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"ok"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	case "delete":
		err := k.kv.Delete(msg.Sender, req.Key)
		if err != nil {
			return api.Message{}, err
		}
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    k.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"status":"deleted"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	}
	return api.Message{}, nil
}

func (k *KVManager) Shutdown(ctx context.Context) error {
	return nil
}
