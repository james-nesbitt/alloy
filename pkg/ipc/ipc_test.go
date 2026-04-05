package ipc

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

type mockRouter struct {
	msgCh chan api.Message
}

func (m *mockRouter) RouteMessage(ctx context.Context, msg api.Message) {
	m.msgCh <- msg
}

func (m *mockRouter) RegisterFrontend(id string, ch chan<- api.Message)                   {}
func (m *mockRouter) RegisterFrontendExt(id string, ch chan<- api.Message, headless bool) {}
func (m *mockRouter) UnregisterFrontend(id string)                                        {}

func TestServerClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	router := &mockRouter{msgCh: make(chan api.Message, 10)}
	server := NewServer(logger, router, nil)

	addr := "127.0.0.1:0"
	go func() {
		_ = server.ListenAndServe(addr)
	}()

	// Wait for server to start
	var serverAddr string
	for i := 0; i < 10; i++ {
		if server.Addr() != nil {
			serverAddr = server.Addr().String()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if serverAddr == "" {
		t.Fatal("server failed to start")
	}

	client, err := Dial(serverAddr, nil)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	defer client.Close()

	msg := api.Message{ID: "test-1", Method: "ping", Sender: "test-client"}
	err = client.Send(msg)
	if err != nil {
		t.Fatalf("client send failed: %v", err)
	}

	// Expect both internal audit events and our ping
	foundRecv := false
	deadline := time.After(1 * time.Second)
	for !foundRecv {
		select {
		case received := <-router.msgCh:
			if received.ID == msg.ID {
				foundRecv = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for message")
		}
	}

	_ = server.Stop()
}

func TestParseAddress(t *testing.T) {
	tests := []struct {
		raw     string
		network string
		address string
	}{
		{"unix:///tmp/test.sock", "unix", "/tmp/test.sock"},
		{"tcp://127.0.0.1:8080", "tcp", "127.0.0.1:8080"},
		{"/tmp/test.sock", "unix", "/tmp/test.sock"},
		{"./test.sock", "unix", "./test.sock"},
		{"127.0.0.1:9000", "tcp", "127.0.0.1:9000"},
	}

	for _, tt := range tests {
		net, addr := ParseAddress(tt.raw)
		if net != tt.network || addr != tt.address {
			t.Errorf("ParseAddress(%s) = (%s, %s), want (%s, %s)", tt.raw, net, addr, tt.network, tt.address)
		}
	}
}
