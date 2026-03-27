package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/james-nesbitt/alloy/api"
)

// MockClient is a mock of the alloy frontend client
type MockClient struct {
	lastTarget  string
	lastMethod  string
	lastPayload []byte
}

func (m *MockClient) Send(ctx context.Context, target, method string, payload []byte) (api.Message, error) {
	m.lastTarget = target
	m.lastMethod = method
	m.lastPayload = payload
	return api.Message{ID: "resp-123", Payload: []byte(`{"status":"ok"}`)}, nil
}

func (m *MockClient) OnMessage(h func(api.Message)) {}
func (m *MockClient) Close() error                  { return nil }
func (m *MockClient) Name() string                  { return "mock" }
func (m *MockClient) Actor() string                 { return "mock" }
func (m *MockClient) Messages() []api.Message       { return nil }

func TestWebFrontendAPI(t *testing.T) {
	mock := &MockClient{}
	wf := &WebFrontend{
		client:     mock,
		eventChans: make([]chan api.Message, 0),
	}

	// 1. Test Discovery API
	t.Run("API Commands", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/commands", nil)
		w := httptest.NewRecorder()
		wf.handleCommands(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
		if mock.lastTarget != "system" || mock.lastMethod != "discovery:list" {
			t.Errorf("Expected system:discovery:list, got %s:%s", mock.lastTarget, mock.lastMethod)
		}
	})

	// 2. Test WS API
	t.Run("WS Connection", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(wf.handleWS))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
		dialer := websocket.Dialer{}
		conn, _, err := dialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to dial WS: %v", err)
		}
		defer conn.Close()

		// Test sending a message through WS
		testMsg := api.Message{
			ID:      "ws-test-1",
			Target:  "project",
			Method:  "create",
			Payload: []byte(`{"name": "test-ws"}`),
		}
		if err := conn.WriteJSON(testMsg); err != nil {
			t.Fatalf("Failed to write to WS: %v", err)
		}

		// Since our handleWS executes target calls in background,
		// we might need a small wait or a more robust mock.
		time.Sleep(100 * time.Millisecond)

		if mock.lastTarget != "project" || mock.lastMethod != "create" {
			t.Errorf("Expected project:create via WS, got %s:%s", mock.lastTarget, mock.lastMethod)
		}
	})
}

func TestWebFrontendBroadcast(t *testing.T) {
	wf := &WebFrontend{
		eventChans: make([]chan api.Message, 0),
	}

	msgChan := make(chan api.Message, 1)
	wf.eventChans = append(wf.eventChans, msgChan)

	testMsg := api.Message{ID: "test-1"}
	wf.broadcast(testMsg)

	select {
	case msg := <-msgChan:
		if msg.ID != "test-1" {
			t.Errorf("Expected ID 'test-1', got %s", msg.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Broadcast failed: timeout")
	}
}
