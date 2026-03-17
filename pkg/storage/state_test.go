package storage

import (
	"bytes"
	"os"
	"testing"
)

func TestFileStateStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "state-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	s, err := NewFileStateStore(tempDir)
	if err != nil {
		t.Fatalf("NewFileStateStore failed: %v", err)
	}

	pluginID := "test-plugin"
	key := "test-key"
	val := []byte("test-value")

	err = s.Set(pluginID, key, val)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := s.Get(pluginID, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !bytes.Equal(got, val) {
		t.Errorf("expected %s, got %s", val, got)
	}

	err = s.Delete(pluginID, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	got, _ = s.Get(pluginID, key)
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestMemoryStateStore(t *testing.T) {
	s := NewMemoryStateStore()
	pluginID := "test-plugin"
	key := "test-key"
	val := []byte("test-value")

	_ = s.Set(pluginID, key, val)
	got, _ := s.Get(pluginID, key)
	if !bytes.Equal(got, val) {
		t.Error("mismatch")
	}

	_ = s.Delete(pluginID, key)
	got, _ = s.Get(pluginID, key)
	if got != nil {
		t.Error("expected nil")
	}

	// Test non-existent get
	got, _ = s.Get("none", "none")
	if got != nil {
		t.Error("expected nil")
	}
}
