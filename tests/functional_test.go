package tests

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/kernel"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

// MockPlugin is a simple plugin for testing.
type MockPlugin struct {
	id string
}

func (m *MockPlugin) ID() string { return m.id }

func (m *MockPlugin) Capabilities() []api.Capability {
	return []api.Capability{{Method: "ping", Description: "Standard echo test"}}
}

func (m *MockPlugin) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	// Simple echo response for testing
	respPayload, _ := json.Marshal(map[string]string{"status": "ok", "echo": string(msg.Payload)})
	return api.Message{
		ID:        msg.ID + "-resp",
		Type:      api.TypeResponse,
		Sender:    m.id,
		Target:    msg.Sender,
		Method:    msg.Method,
		Payload:   respPayload,
		Timestamp: time.Now().Unix(),
	}, nil
}

func (m *MockPlugin) Shutdown(ctx context.Context) error { return nil }

func TestFunctionalMessageFlow(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// 1. Initialize Kernel
	k, _ := kernel.New(logger, storage.NewMemoryStateStore(), "", "")
	if err := k.Start(ctx); err != nil {
		t.Fatalf("failed to start kernel: %v", err)
	}
	defer k.Shutdown(ctx)

	// 2. Register Mock Plugin
	p := &MockPlugin{id: "test-plugin"}
	k.RegisterPlugin(p)

	// 3. Logic for "Mock Frontend" sending a message
	respChan := make(chan api.Message, 1)
	k.RegisterFrontend("frontend-1", respChan)

	requestPayload, _ := json.Marshal(map[string]string{"cmd": "hello"})
	msg := api.Message{
		ID:        "msg-1",
		Type:      api.TypeRequest,
		Sender:    "frontend-1",
		Target:    "test-plugin",
		Method:    "ping",
		Payload:   requestPayload,
		Timestamp: time.Now().Unix(),
	}

	t.Log("Sending message from mock frontend to plugin via kernel")
	k.RouteMessage(ctx, msg)

	// 4. Wait for response back at the "frontend"
	select {
	case resp := <-respChan:
		t.Logf("Received response: %s", string(resp.Payload))
		if resp.Sender != "test-plugin" {
			t.Errorf("expected response from test-plugin, got %s", resp.Sender)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for response from plugin")
	}

	// 5. Test internal kernel messages
	k.RouteMessage(ctx, api.Message{
		ID:     "ping-kernel",
		Type:   api.TypeRequest,
		Sender: "frontend-1",
		Target: "kernel",
		Method: "ping",
	})
	select {
	case resp := <-respChan:
		if resp.Sender != "kernel" {
			t.Errorf("expected response from kernel, got %s", resp.Sender)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for kernel ping response")
	}

	k.RouteMessage(ctx, api.Message{
		Target: "kernel",
		Method: "audit",
	})
	k.RouteMessage(ctx, api.Message{
		Target: "kernel",
		Method: "invalid",
	})
}
