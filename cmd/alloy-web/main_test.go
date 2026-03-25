package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

	// 2. Test Send API (Legacy or direct)
	t.Run("API Send", func(t *testing.T) {
		sendReq := `{"target": "project", "method": "create", "payload": "{\"name\": \"test\"}"}`
		req := httptest.NewRequest("POST", "/api/send", strings.NewReader(sendReq))
		w := httptest.NewRecorder()
		wf.handleSend(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
		if mock.lastTarget != "project" || mock.lastMethod != "create" {
			t.Errorf("Expected project:create, got %s:%s", mock.lastTarget, mock.lastMethod)
		}
		if !strings.Contains(string(mock.lastPayload), "test") {
			t.Errorf("Expected payload to contain 'test', got %s", string(mock.lastPayload))
		}
	})

	// 3. Test SSE (Keepalive)
	t.Run("API Events", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/events", nil)
		w := httptest.NewRecorder()

		// Use a context that cancels after 50ms to end the SSE loop
		ctx, cancel := context.WithTimeout(req.Context(), 50*time.Millisecond)
		defer cancel()
		req = req.WithContext(ctx)

		// We need a custom ResponseWriter that flushes to satisfy handleEvents
		// httptest.ResponseRecorder implements Flusher as well.

		// Actually, handleEvents blocks until context done or message.
		// Since we have a keepalive timer at 15s, it will block for 50ms and return.
		wf.handleEvents(w, req)

		if w.Header().Get("Content-Type") != "text/event-stream" {
			t.Errorf("Expected event-stream header, got %s", w.Header().Get("Content-Type"))
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
