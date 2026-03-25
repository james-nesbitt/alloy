package tests

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/ipc"
	"github.com/james-nesbitt/alloy/pkg/kernel"
	"github.com/james-nesbitt/alloy/pkg/security/identity"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

func TestMTLSMessageFlow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Setup Identity Store (Temporary)
	tmpDir, _ := os.MkdirTemp("", "alloy-test-*")
	defer os.RemoveAll(tmpDir)
	store := identity.NewStore(tmpDir)

	ca, err := store.InitializeMachine()
	if err != nil {
		t.Fatalf("failed to init machine: %v", err)
	}

	serverPair, _ := store.CreateInstanceIdentity(ca, "test-instance")
	serverTLS, _ := store.GetServerTLSConfig(ca, serverPair)
	clientTLS, _ := store.GetClientTLSConfig(ca, "test-client")

	// 2. Start Kernel
	k, _ := kernel.New(logger, storage.NewMemoryStateStore(), "", "")
	if err := k.Start(ctx); err != nil {
		t.Fatalf("failed to start kernel: %v", err)
	}
	defer k.Stop(ctx)

	// 3. Start IPC Server
	server := ipc.NewServer(logger, k, serverTLS)
	go func() {
		if err := server.ListenAndServe("127.0.0.1:0"); err != nil {
		}
	}()

	var addr string
	for i := 0; i < 10; i++ {
		if server.Addr() != nil {
			addr = server.Addr().String()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 4. Connect Client
	client, err := ipc.Dial(addr, clientTLS)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	// 5. Send Ping
	msg := api.Message{
		ID:        "ping-mtls",
		Type:      api.TypeRequest,
		Sender:    "test-client",
		Target:    "kernel",
		Method:    "ping",
		Timestamp: time.Now().Unix(),
	}

	resp, err := client.Call(ctx, msg)
	if err != nil {
		t.Fatalf("mtls call failed: %v", err)
	}

	if resp.Sender != "kernel" {
		t.Errorf("expected sender kernel, got %s", resp.Sender)
	}
}
