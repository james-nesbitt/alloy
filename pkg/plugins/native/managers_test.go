package native

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jnesbitt/alloy-go/api"
)

type mockKVStore struct{}

func (m *mockKVStore) Get(pID, k string) ([]byte, error) { return nil, nil }
func (m *mockKVStore) Set(pID, k string, v []byte) error { return nil }
func (m *mockKVStore) Delete(pID, k string) error        { return nil }

func TestCommandManager(t *testing.T) {
	mgr := NewCommandManager()
	if mgr.ID() != "plugin-command-manager" {
		t.Errorf("expected ID plugin-command-manager, got %s", mgr.ID())
	}
	ctx := context.Background()
	_, err := mgr.HandleMessage(ctx, api.Message{Method: "register", Sender: "test", Payload: []byte("[]")})
	if err != nil {
		t.Errorf("HandleMessage register failed: %v", err)
	}
	_ = mgr.Shutdown(ctx)
}

func TestEventManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mgr := NewEventManager(logger)
	if mgr.ID() != "plugin-events" {
		t.Errorf("expected ID plugin-events, got %s", mgr.ID())
	}
	ctx := context.Background()
	_, err := mgr.HandleMessage(ctx, api.Message{Method: "subscribe", Sender: "test", Payload: []byte(`{"topic":"test"}`)})
	if err != nil {
		t.Errorf("HandleMessage subscribe failed: %v", err)
	}
	_ = mgr.Shutdown(ctx)
}

func TestServiceManagers(t *testing.T) {
	ctx := context.Background()

	cache := NewCacheManager()
	if cache.ID() != "plugin-cache" {
		t.Error("cache ID mismatch")
	}
	_, _ = cache.HandleMessage(ctx, api.Message{})
	_ = cache.Shutdown(ctx)

	doc := NewDocStore()
	if doc.ID() != "plugin-doc" {
		t.Error("doc ID mismatch")
	}
	_, _ = doc.HandleMessage(ctx, api.Message{})
	_ = doc.Shutdown(ctx)

	net := NewNetworkManager()
	if net.ID() != "plugin-network" {
		t.Error("net ID mismatch")
	}
	_, _ = net.HandleMessage(ctx, api.Message{})
	_ = net.Shutdown(ctx)

	storage := NewStorageManager()
	if storage.ID() != "plugin-storage" {
		t.Error("storage ID mismatch")
	}
	_, _ = storage.HandleMessage(ctx, api.Message{})
	_ = storage.Shutdown(ctx)
}

func TestKVManager(t *testing.T) {
	mgr := NewKVManager(&mockKVStore{})
	if mgr.ID() != "plugin-kv" {
		t.Errorf("expected ID plugin-kv, got %s", mgr.ID())
	}
	if len(mgr.Capabilities()) == 0 {
		t.Error("expected capabilities")
	}

	ctx := context.Background()
	_, err := mgr.HandleMessage(ctx, api.Message{Method: "get", Sender: "test"})
	if err != nil {
		t.Errorf("HandleMessage get failed: %v", err)
	}
	_, err = mgr.HandleMessage(ctx, api.Message{Method: "set", Sender: "test"})
	if err != nil {
		t.Errorf("HandleMessage set failed: %v", err)
	}
	_ = mgr.Shutdown(ctx)
}

func TestIAMManager(t *testing.T) {
	mgr := NewIAMManager()
	if mgr.ID() != "plugin-iam" {
		t.Errorf("expected ID plugin-iam, got %s", mgr.ID())
	}
	ctx := context.Background()
	_, err := mgr.HandleMessage(ctx, api.Message{Method: "check", Sender: "test"})
	if err != nil {
		t.Errorf("HandleMessage check failed: %v", err)
	}
	_ = mgr.Shutdown(ctx)
}

func TestSecretManager(t *testing.T) {
	mgr := NewSecretManager()
	if mgr.ID() != "plugin-secrets" {
		t.Errorf("expected ID plugin-secrets, got %s", mgr.ID())
	}
	ctx := context.Background()
	_, err := mgr.HandleMessage(ctx, api.Message{Method: "get", Sender: "test"})
	if err != nil {
		t.Errorf("HandleMessage get failed: %v", err)
	}
	_ = mgr.Shutdown(ctx)
}

func TestHealthManager(t *testing.T) {
	mgr := NewHealthManager()
	if mgr.ID() != "plugin-health" {
		t.Errorf("expected ID plugin-health, got %s", mgr.ID())
	}
	ctx := context.Background()
	_, err := mgr.HandleMessage(ctx, api.Message{Method: "ping", Sender: "test"})
	if err != nil {
		t.Errorf("HandleMessage ping failed: %v", err)
	}
	_ = mgr.Shutdown(ctx)
}

func TestTaskRunner(t *testing.T) {
	mgr := NewTaskRunner()
	if mgr.ID() != "plugin-tasks" {
		t.Errorf("expected ID plugin-tasks, got %s", mgr.ID())
	}
	ctx := context.Background()
	_, err := mgr.HandleMessage(ctx, api.Message{Method: "run", Sender: "test"})
	if err != nil {
		t.Errorf("HandleMessage run failed: %v", err)
	}
	_ = mgr.Shutdown(ctx)
}
