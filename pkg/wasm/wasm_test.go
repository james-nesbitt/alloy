package wasm

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
)

func TestWasmGoOrchestration(t *testing.T) {
	// This test verifies that the Go structures are correctly initialized.
	// Real WASM execution tests will be added once we have a stable build process for plugins.
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	kv := NewMemoryStateStore()

	r, err := NewRuntime(ctx, logger, kv)
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}
	defer r.Close(ctx)

	if r == nil {
		t.Fatal("runtime is nil")
	}

	hostMod, err := r.InstantiateAlloyHost(ctx)
	if err != nil {
		t.Fatalf("failed to instantiate host module: %v", err)
	}
	if hostMod == nil {
		t.Fatal("host module is nil")
	}
}

// MemoryStateStore satisfies the wasm.KVStore interface for testing.
type MemoryStateStore struct {
	mu   sync.RWMutex
	data map[string]map[string][]byte
}

func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{data: make(map[string]map[string][]byte)}
}

func (m *MemoryStateStore) Get(pID, k string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.data[pID]; ok {
		return p[k], nil
	}
	return nil, nil
}

func (m *MemoryStateStore) Set(pID, k string, v []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[pID]; !ok {
		m.data[pID] = make(map[string][]byte)
	}
	m.data[pID][k] = v
	return nil
}
