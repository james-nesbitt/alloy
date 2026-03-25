package kernel

import (
	"log/slog"
	"os"
	"testing"

	"github.com/james-nesbitt/alloy/pkg/storage"
)

func TestKernelBridge(t *testing.T) {
	dataDir, _ := os.MkdirTemp("", "kernel-bridge-test-*")
	defer os.RemoveAll(dataDir)

	kv, _ := storage.NewFileStateStore(dataDir + "/state")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	k, _ := New(logger, kv, dataDir, "")

	// Create a bridge for a fake kernel component "test-comp"
	bridge := k.NewBridge("test-comp")

	// Test KV persistence
	val := []byte("alloy-data")
	if !bridge.KVSet("my-key", val) {
		t.Fatal("bridge.KVSet failed")
	}

	readVal, ok := bridge.KVGet("my-key")
	if !ok {
		t.Fatal("bridge.KVGet returned false")
	}
	if string(readVal) != string(val) {
		t.Errorf("read value %s, expected %s", string(readVal), string(val))
	}

	// Test Eventing (asynchronous, so we just check no error for now)
	bridge.Publish("test-topic", map[string]string{"foo": "bar"})

	// Test synchronous Call (mock-like)
	resp, err := bridge.Call("kv", "list", map[string]string{"prefix": ""})
	if err != nil {
		t.Fatalf("bridge.Call error: %v", err)
	}
	if resp.ID == "" {
		t.Fatal("bridge.Call returned empty message ID")
	}
}
