package kernel

import (
	"context"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

// CacheManager handles high-speed transient data.
type CacheManager struct{}

func NewCacheManager() *CacheManager { return &CacheManager{} }
func (c *CacheManager) ID() string   { return "cache" }
func (c *CacheManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "get", Description: "Get cached value"},
		{Method: "set", Description: "Set cached value with TTL"},
	}
}
func (c *CacheManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	return api.Message{
		ID:        msg.ID + "-resp",
		Type:      api.TypeResponse,
		Sender:    c.ID(),
		Target:    msg.Sender,
		Payload:   []byte(`{"status":"ok"}`),
		Timestamp: time.Now().Unix(),
	}, nil
}
func (c *CacheManager) Shutdown(ctx context.Context) error { return nil }

// DocStore handles indexed documents.
type DocStore struct{}

func NewDocStore() *DocStore   { return &DocStore{} }
func (d *DocStore) ID() string { return "doc" }
func (d *DocStore) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "find", Description: "Query documents"},
		{Method: "insert", Description: "Insert document"},
	}
}
func (d *DocStore) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	return api.Message{
		ID:        msg.ID + "-resp",
		Type:      api.TypeResponse,
		Sender:    d.ID(),
		Target:    msg.Sender,
		Payload:   []byte(`{"results":[]}`),
		Timestamp: time.Now().Unix(),
	}, nil
}
func (d *DocStore) Shutdown(ctx context.Context) error { return nil }

// NetworkManager handles policy-enforced network access.
type NetworkManager struct{}

func NewNetworkManager() *NetworkManager { return &NetworkManager{} }
func (n *NetworkManager) ID() string     { return "network" }
func (n *NetworkManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "fetch", Description: "Perform a policy-checked HTTP request"},
	}
}
func (n *NetworkManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	return api.Message{
		ID:        msg.ID + "-resp",
		Type:      api.TypeResponse,
		Sender:    n.ID(),
		Target:    msg.Sender,
		Payload:   []byte(`{"status":403,"error":"policy-denied"}`),
		Timestamp: time.Now().Unix(),
	}, nil
}
func (n *NetworkManager) Shutdown(ctx context.Context) error { return nil }

// StorageManager handles virtual filesystem access.
type StorageManager struct{}

func NewStorageManager() *StorageManager { return &StorageManager{} }
func (s *StorageManager) ID() string     { return "storage" }
func (s *StorageManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "list", Description: "List files in a scoped directory"},
	}
}
func (s *StorageManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	return api.Message{
		ID:        msg.ID + "-resp",
		Type:      api.TypeResponse,
		Sender:    s.ID(),
		Target:    msg.Sender,
		Payload:   []byte(`{"files":[]}`),
		Timestamp: time.Now().Unix(),
	}, nil
}
func (s *StorageManager) Shutdown(ctx context.Context) error { return nil }
