package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/kernel"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

func TestLibrarianSemanticSearch(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "librarian-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	kv, _ := storage.NewFileStateStore(filepath.Join(tempDir, "storage"))

	cwd, _ := os.Getwd()
	projectRoot := filepath.Dir(cwd)
	pluginsDir := filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins")

	// Initialize Kernel
	k, err := kernel.New(logger, kv, filepath.Join(tempDir, "kernel_data"), ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer k.Shutdown(context.Background())
	k.SetInsecure(true) // Disable RBAC for simpler test

	// Load necessary plugins
	// 1. AI Plugin
	aiBytes, _ := os.ReadFile(filepath.Join(pluginsDir, "ai.wasm"))
	err = k.RegisterWASMPluginAtScale("ai", aiBytes, 256, 100, []api.Capability{
		{Method: "ai:embed", Description: "Embed"},
	}, false, false, false)
	if err != nil {
		t.Fatalf("failed to load ai plugin: %v", err)
	}

	// 2. Buffer Manager
	bufBytes, _ := os.ReadFile(filepath.Join(pluginsDir, "buffer.wasm"))
	err = k.RegisterWASMPluginAtScale("buffer", bufBytes, 256, 100, []api.Capability{
		{Method: "buffer:create", Description: "Create"},
		{Method: "buffer:read", Description: "Read"},
		{Method: "buffer:write", Description: "Write"},
	}, false, false, false)
	if err != nil {
		t.Fatalf("failed to load buffer plugin: %v", err)
	}

	// 3. Librarian
	libBytes, _ := os.ReadFile(filepath.Join(pluginsDir, "librarian.wasm"))
	err = k.RegisterWASMPluginAtScale("librarian", libBytes, 256, 100, []api.Capability{
		{Method: "librarian:search", Description: "Search"},
		{Method: "librarian:index-buffer", Description: "Index"},
	}, false, true, false)
	if err != nil {
		t.Fatalf("failed to load librarian plugin: %v", err)
	}

	// 4. Omni-Palette
	omniBytes, _ := os.ReadFile(filepath.Join(pluginsDir, "omni-palette.wasm"))
	err = k.RegisterWASMPluginAtScale("omni-palette", omniBytes, 256, 100, []api.Capability{
		{Method: "omni:search", Description: "Unified Search"},
	}, false, false, false)
	if err != nil {
		t.Fatalf("failed to load omni-palette plugin: %v", err)
	}

	// Wait for plugins to initialize
	time.Sleep(1 * time.Second)

	// Register a response channel for the test client
	respChan := make(chan api.Message, 16)
	frontendID := "test-client"
	k.RegisterFrontend(frontendID, respChan)

	waitForResponse := func(expectedID string) api.Message {
		deadline := time.After(10 * time.Second)
		var lastMsg api.Message
		for {
			select {
			case msg := <-respChan:
				lastMsg = msg
				if msg.Target == frontendID {
					if msg.ID == expectedID || msg.ID == expectedID+"-response" || msg.ID == expectedID+"-resp" {
						return msg
					}
				}
			case <-deadline:
				t.Fatalf("timeout waiting for %s response (last msg type=%s method=%s sender=%s)", expectedID, lastMsg.Type, lastMsg.Method, lastMsg.Sender)
			}
		}
	}

	// 5. Create a buffer
	createBufMsg := api.Message{
		ID:      "create-buf-1",
		Type:    api.TypeRequest,
		Sender:  frontendID,
		Target:  "buffer",
		Method:  "buffer:create",
		Payload: []byte(`{"name": "test.txt"}`),
	}
	k.RouteMessage(context.Background(), createBufMsg)
	createResp := waitForResponse("create-buf-1")

	var bufInfo struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createResp.Payload, &bufInfo); err != nil {
		t.Fatalf("failed to parse create-buffer response: %v payload=%s", err, string(createResp.Payload))
	}

	content := "Machine learning is a field of artificial intelligence."
	// TWFjaGluZSBsZWFybmluZyBpcyBhIGZpZWxkIG9mIGFydGlmaWNpYWwgaW50ZWxsaWdlbmNlLg== is "Machine learning is a field of artificial intelligence."
	writeMsg := api.Message{
		ID:      "write-buf-1",
		Type:    api.TypeRequest,
		Sender:  frontendID,
		Target:  "buffer",
		Method:  "buffer:write",
		Payload: []byte(`{"id": "` + bufInfo.ID + `", "content": "TWFjaGluZSBsZWFybmluZyBpcyBhIGZpZWxkIG9mIGFydGlmaWNpYWwgaW50ZWxsaWdlbmNlLg=="}`),
	}
	k.RouteMessage(context.Background(), writeMsg)
	waitForResponse("write-buf-1")

	// 6. Ingest it into the Librarian
	ingestMsg := api.Message{
		ID:      "manual-ingest-1",
		Type:    api.TypeRequest,
		Sender:  frontendID,
		Target:  "librarian",
		Method:  "librarian:index-buffer",
		Payload: []byte(`{"id": "` + bufInfo.ID + `"}`),
	}
	k.RouteMessage(context.Background(), ingestMsg)
	waitForResponse("manual-ingest-1")

	// Wait for indexing to complete
	time.Sleep(500 * time.Millisecond)

	// 7. Test search
	searchMsg := api.Message{
		ID:      "omni-lib-test",
		Type:    api.TypeRequest,
		Sender:  frontendID,
		Target:  "omni-palette",
		Method:  "omni:search",
		Payload: []byte(`{"query": "` + content + `", "limit": 5}`),
	}
	k.RouteMessage(context.Background(), searchMsg)

	resp := waitForResponse(searchMsg.ID)
	fmt.Printf("Search response: %s\n", string(resp.Payload))

	if strings.Contains(string(resp.Payload), "error") {
		t.Fatalf("search returned error: %s", string(resp.Payload))
	}

	if !strings.Contains(string(resp.Payload), "buf-") {
		t.Errorf("expected to find semantic result referencing 'buf-', got: %s", string(resp.Payload))
	}
}
