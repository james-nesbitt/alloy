package tests

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/ipc"
	"github.com/jnesbitt/alloy-go/pkg/kernel"
)

func TestIPCMessageFlow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Start Kernel
	k := kernel.New(logger)
	if err := k.Start(ctx); err != nil {
		t.Fatalf("failed to start kernel: %v", err)
	}
	defer k.Stop(ctx)

	// 2. Start IPC Server on a random port
	server := ipc.NewServer(logger, k, nil)
	go func() {
		if err := server.ListenAndServe("127.0.0.1:0"); err != nil {
			// server.Stop() will cause an error here, which is fine
		}
	}()

	// Wait for server to start and get address
	var addr string
	for i := 0; i < 10; i++ {
		if server.Addr() != nil {
			addr = server.Addr().String()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if addr == "" {
		t.Fatal("failed to get server address")
	}
	t.Logf("Server listening on %s", addr)

	// 3. Connect Client
	client, err := ipc.Dial(addr, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	// 4. Send Ping
	msg := api.Message{
		ID:        "test-ping",
		Type:      api.TypeRequest,
		Sender:    "client",
		Target:    "kernel",
		Method:    "ping",
		Timestamp: time.Now().Unix(),
	}

	resp, err := client.Call(ctx, msg)
	if err != nil {
		t.Fatalf("ping call failed: %v", err)
	}

	t.Logf("Received response: %s", string(resp.Payload))
	if resp.Sender != "kernel" {
		t.Errorf("expected sender kernel, got %s", resp.Sender)
	}
}
