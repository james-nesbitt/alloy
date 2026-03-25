package kernel

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

func BenchmarkKernelRouting(b *testing.B) {
	os.Setenv("ALLOY_TELEMETRY_SILENT", "true")
	dataDir, _ := os.MkdirTemp("", "bench-kernel-*")
	defer os.RemoveAll(dataDir)

	kv, _ := storage.NewFileStateStore(dataDir + "/state")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil)) // No logging for bench

	k, _ := New(logger, kv, dataDir, "")
	k.SetInsecure(true) // Disable RBAC for baseline routing bench

	// Create a larger dummy receiver
	rx := make(chan api.Message, 1000)
	k.RegisterFrontend("bench-rx", rx)

	ctx := context.Background()
	msg := api.Message{
		ID:     "bench-1",
		Sender: "bench-tx",
		Target: "bench-rx",
		Method: "nop",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k.RouteMessage(ctx, msg)
		select {
		case <-rx:
		case <-time.After(50 * time.Millisecond):
			b.Fatalf("timeout waiting for message %d", i)
		}
	}
}

func BenchmarkKV_Integrated(b *testing.B) {
	os.Setenv("ALLOY_TELEMETRY_SILENT", "true")
	dataDir, _ := os.MkdirTemp("", "bench-kv-*")
	defer os.RemoveAll(dataDir)

	kvProvider, _ := storage.NewFileStateStore(dataDir + "/state")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	k, _ := New(logger, kvProvider, dataDir, "")

	ctx := context.Background()
	req := api.Message{
		ID:      "req-1",
		Sender:  "bench",
		Target:  "kv",
		Method:  "set",
		Payload: []byte(`{"key":"bench-key","value":"some-data"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := k.HandleMessageSync(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKV_Bridge(b *testing.B) {
	os.Setenv("ALLOY_TELEMETRY_SILENT", "true")
	dataDir, _ := os.MkdirTemp("", "bench-bridge-*")
	defer os.RemoveAll(dataDir)

	kvProvider, _ := storage.NewFileStateStore(dataDir + "/state")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	k, _ := New(logger, kvProvider, dataDir, "")
	bridge := k.NewBridge("bench-plugin")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ok := bridge.KVSet("bench-key", []byte("some-data"))
		if !ok {
			b.Fatal("kv set failed")
		}
	}
}
