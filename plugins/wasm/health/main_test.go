package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/pkg/wasm/guest"
)

func TestHealthPlugin(t *testing.T) {
	// 1. Create mock environment
	plugin, mock := guest.NewMockPlugin("health")

	// 2. Register the status handler (same logic as main_wit.go)
	plugin.RegisterMethod("status", "Get status", func(msg guest.AlloyMessage) *guest.AlloyMessage {
		status := map[string]string{
			"status": "healthy",
			"uptime": "mock-test",
			"source": "health-plugin",
		}
		payload, _ := json.Marshal(status)
		return &guest.AlloyMessage{
			Id:      msg.Id + "-resp",
			Method:  msg.Method,
			Payload: payload,
			Target:  msg.Target,
		}
	})

	// 3. Serve the plugin in a separate goroutine
	go plugin.Serve()

	// 4. Push a request message
	req := guest.AlloyMessage{
		Id:     "msg-1",
		Method: "status",
		Sender: "tester-client",
	}
	mock.PushMessage(req)

	// 5. Polling wait for the response (unit test speed: <10ms usually)
	var resp guest.AlloyMessage
	var ok bool
	timeout := time.After(500 * time.Millisecond)
	
	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for plugin response")
		default:
			resp, ok = mock.GetResponse("msg-1-resp")
			if ok {
				goto verified
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

verified:
	// 6. Verify response data
	var status map[string]string
	if err := json.Unmarshal(resp.Payload, &status); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if status["status"] != "healthy" {
		t.Errorf("Expected status healthy, got %s", status["status"])
	}
	
	if status["source"] != "health-plugin" {
		t.Errorf("Wrong source: %s", status["source"])
	}
}

func TestKVIntegrationMock(t *testing.T) {
	plugin, mock := guest.NewMockPlugin("health")
	
	// Test KV persistence via SDK
	plugin.KVSet("test-key", []byte("test-value"))
	
	val, ok := plugin.KVGet("test-key")
	if !ok || string(val) != "test-value" {
		t.Errorf("KV storage failed in mock: got %s, ok=%v", string(val), ok)
	}
	
	// Verify it reached the mock host's memory
	if string(mock.KV["test-key"]) != "test-value" {
		t.Errorf("KV didn't reach mock host state")
	}
}
