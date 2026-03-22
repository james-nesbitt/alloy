package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jnesbitt/alloy-go/pkg/wasm/guest"
)

func TestHealthPlugin(t *testing.T) {
	// 1. Initialize our plugin mock environment
	wasm.ResetMock()
	
	p := wasm.New("plugin-health").
		WithCapability("status", "Get status", "h s").
		Handle("status", func(msg wasm.Message) wasm.Message {
			return wasm.Reply(msg, map[string]string{"status": "ok"})
		})
	
	// 2. Simulate a status message
	input := wasm.Message{
		ID:     "123",
		Method: "status",
		Sender: "client",
		Target: "plugin-health",
	}
	
	output := p.MockSimulate(input)
	
	// 3. Verify the output
	if output.Type != "response" {
		t.Errorf("Expected response message, got %s", output.Type)
	}
	
	var res map[string]string
	if err := json.Unmarshal(output.Payload, &res); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}
	
	if res["status"] != "ok" {
		t.Errorf("Expected status ok, got %s", res["status"])
	}
}

func TestStorageMock(t *testing.T) {
	wasm.ResetMock()
	
	store := wasm.NewKVStore[string]("test-prefix")
	err := store.Set("key1", "value1")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	
	val, err := store.Get("key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	
	if val != "value1" {
		t.Errorf("Expected value1, got %s", val)
	}
	
	// Verify it's actually in the underlying mock map with premium prefixing
	underlying := wasm.GetKV()
	data := underlying["test-prefix:key1"]
	if data == nil {
		t.Errorf("Prefixing failed, key not found in mock store")
	}
}

func TestNetworkMock(t *testing.T) {
	wasm.ResetMock()
	
	// Mock an external API
	wasm.SetFetchHandler(func(req wasm.FetchRequest) (*wasm.FetchResponse, error) {
		if req.URL == "https://api.example.com/status" {
			return &wasm.FetchResponse{
				Status: 200,
				Body:   []byte(`{"status":"up"}`),
			}, nil
		}
		return nil, fmt.Errorf("wrong url")
	})
	
	type statusRes struct {
		Status string `json:"status"`
	}
	
	res, err := wasm.GetJSON[statusRes]("https://api.example.com/status", nil)
	if err != nil {
		t.Fatalf("GetJSON failed: %v", err)
	}
	
	if res.Status != "up" {
		t.Errorf("Expected status up, got %s", res.Status)
	}
}
