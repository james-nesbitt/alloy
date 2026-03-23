package tests

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/kernel"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

func TestAIDirectBufferInteraction(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "ai-direct-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Set up logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Set up storage
	storagePath := filepath.Join(tempDir, "storage")
	kv, err := storage.NewFileStateStore(storagePath)
	if err != nil {
		t.Fatal(err)
	}

	// Create WIT kernel
	k, err := kernel.NewWITKernel(logger, kv, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Shutdown(context.Background())

	cwd, _ := os.Getwd()
	projectRoot := filepath.Dir(cwd)

	// Load Buffer plugin
	bufferWasm, err := os.ReadFile(filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/buffer.wasm"))
	if err != nil {
		t.Fatalf("failed to read buffer.wasm (run 'just build-plugin buffer'): %v", err)
	}
	err = k.RegisterWASMPlugin("buffer", bufferWasm, []api.Capability{
		{Method: "create", Description: "Create buffer"},
		{Method: "read", Description: "Read buffer"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Load AI plugin
	aiWasm, err := os.ReadFile(filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/ai.wasm"))
	if err != nil {
		t.Fatalf("failed to read ai.wasm (run 'just build-plugin ai'): %v", err)
	}
	err = k.RegisterWASMPlugin("ai", aiWasm, []api.Capability{
		{Method: "summarize-buffer", Description: "Summarize buffer"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for plugins to initialize
	time.Sleep(500 * time.Millisecond)

	// 1. Create a buffer
	frontendCh := make(chan api.Message, 10)
	k.RegisterFrontend("test-user", frontendCh)

	createMsg := api.Message{
		ID:      "req-create-buf",
		Type:    api.TypeRequest,
		Method:  "create",
		Sender:  "test-user",
		Target:  "buffer",
		Payload: json.RawMessage(`{"name":"test.txt","content":"YWxsb3kgaXMgYXdlc29tZQ==","type":"file"}`), // "alloy is awesome"
	}
	go k.RouteMessage(context.Background(), createMsg)

	var bufferID string
	select {
	case resp := <-frontendCh:
		var buf struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(resp.Payload, &buf); err != nil {
			t.Fatal(err)
		}
		bufferID = buf.ID
		t.Logf("Created buffer: %s", bufferID)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for buffer creation")
	}

	// 2. Request summarization of that buffer from the AI plugin
	summarizeMsg := api.Message{
		ID:      "req-summarize",
		Type:    api.TypeRequest,
		Method:  "summarize-buffer",
		Sender:  "test-user",
		Target:  "ai",
		Payload: json.RawMessage(`{"id":"` + bufferID + `"}`),
	}
	go k.RouteMessage(context.Background(), summarizeMsg)

	select {
	case resp := <-frontendCh:
		if resp.ID != "req-summarize-resp" {
			t.Errorf("Unexpected response ID: %s", resp.ID)
		}
		var aiResp struct {
			Response string `json:"response"`
		}
		if err := json.Unmarshal(resp.Payload, &aiResp); err != nil {
			t.Fatal(err)
		}
		t.Logf("AI Response: %s", aiResp.Response)
		
		// The mock provider in AI plugin currently returns "Mock AI response with system context ... to: Summarize..."
		// or just "Mock AI response to: Summarize..." if no context
		if aiResp.Response == "" {
			t.Fatal("AI response empty")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for AI summarization (direct interaction)")
	}

	t.Log("AI Direct Buffer Interaction test passed!")
}
