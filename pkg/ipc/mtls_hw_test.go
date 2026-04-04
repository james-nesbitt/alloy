package ipc

import (
	"context"
	"encoding/pem"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/security/identity"
	"github.com/james-nesbitt/alloy/pkg/security/pki"
)

type hwMockRouter struct {
	msgCh chan api.Message
}

func (m *hwMockRouter) RouteMessage(ctx context.Context, msg api.Message) {
	m.msgCh <- msg
}

func (m *hwMockRouter) RegisterFrontend(id string, ch chan<- api.Message) {}
func (m *hwMockRouter) RegisterFrontendExt(id string, ch chan<- api.Message, headless bool) {}
func (m *hwMockRouter) UnregisterFrontend(id string) {}

func TestHardwareMTLSDirect(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// 1. Setup Identity Store with TPM simulation
	tmpDir, _ := os.MkdirTemp("", "alloy-hw-ipc-test-*")
	defer os.RemoveAll(tmpDir)
	store := identity.NewStore(tmpDir)
	store.SetHardware("tpm") // Use our registered TPM provider

	ca, err := store.InitializeMachine()
	if err != nil {
		t.Fatalf("failed to init machine: %v", err)
	}

	serverPair, err := store.CreateInstanceIdentity(ca, "hw-ipc-server")
	if err != nil {
		t.Fatalf("failed to create server pair: %v", err)
	}

	serverTLS, err := store.GetServerTLSConfig(ca, serverPair)
	if err != nil {
		t.Fatalf("failed to get server tls: %v", err)
	}

	clientTLS, err := store.GetClientTLSConfig(ca, "hw-ipc-client")
	if err != nil {
		t.Fatalf("failed to get client tls: %v", err)
	}

	// 2. Start IPC Server
	router := &hwMockRouter{msgCh: make(chan api.Message, 10)}
	server := NewServer(logger, router, serverTLS)

	go func() {
		_ = server.ListenAndServe("127.0.0.1:0")
	}()

	// Wait for server to start
	var serverAddr string
	for i := 0; i < 10; i++ {
		if server.Addr() != nil {
			serverAddr = server.Addr().String()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if serverAddr == "" {
		t.Fatal("server failed to start")
	}

	// 3. Dial Client with Hardware Credentials
	client, err := Dial(serverAddr, clientTLS)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	defer client.Close()

	// 4. Verify connectivity
	msg := api.Message{ID: "test-hw-1", Method: "ping", Sender: "hw-client"}
	err = client.Send(msg)
	if err != nil {
		t.Fatalf("client send failed: %v", err)
	}

	foundResp := false
	for !foundResp {
		select {
		case received := <-router.msgCh:
			if received.ID == msg.ID {
				foundResp = true
			} else {
				slog.Debug("skipping non-target message", "id", received.ID, "method", received.Method)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for message")
		}
	}

	_ = server.Stop()
}

func TestHardwarePEMMismatch(t *testing.T) {
	// Verify that if we have a hardware key PEM but the provider is not registered, it fails
	// We'll create a bogus PEM block
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "ALOY HARDWARE KEY",
		Headers: map[string]string{
			"Provider": "missing-chip",
		},
		Bytes: []byte("bogus-id"),
	})
	certPEM := []byte("fake-cert")

	_, err := pki.LoadTLSCertificate(certPEM, keyPEM)
	if err == nil {
		t.Error("expected error loading hardware key with missing provider")
	}
}
