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

func TestOmniPaletteSearch(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "omni-palette-test")
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

	// 1. Load Indexer
	indexBytes, _ := os.ReadFile(filepath.Join(pluginsDir, "index.wasm"))
	err = k.RegisterWASMPluginAtScale("index", indexBytes, 256, 100, []api.Capability{
		{Method: "knowledge:ingest", Description: "Ingest doc"},
		{Method: "knowledge:search", Description: "Search doc"},
	}, false, false, false)
	if err != nil {
		t.Fatalf("failed to load index plugin: %v", err)
	}

	// 2. Load Omni-Palette
	omniBytes, _ := os.ReadFile(filepath.Join(pluginsDir, "omni-palette.wasm"))
	err = k.RegisterWASMPluginAtScale("omni-palette", omniBytes, 256, 100, []api.Capability{
		{Method: "omni:search", Description: "Unified Search"},
	}, false, false, false)

	if err != nil {
		t.Fatalf("failed to load omni-palette plugin: %v", err)
	}

	// Wait for plugins to initialize
	time.Sleep(500 * time.Millisecond)

	// 3. Ingest some content into the Knowledge Graph
	ingestMsg := api.Message{
		ID:     "ingest-1",
		Type:   api.TypeRequest,
		Sender: "test-client",
		Target: "index",
		Method: "knowledge:ingest",
		Payload: []byte(`{
 			"id": "doc-omni-test",
 			"path": "test://omni-doc",
 			"content": "Alloy Omni-Palette is a unified search interface for commands and knowledge.",
 			"tags": ["omni", "testing"]
 		}`),
	}
	k.RouteMessage(context.Background(), ingestMsg)

	// Wait for ingestion to complete (it's async thru the router)
	time.Sleep(500 * time.Millisecond)

	// 4. Test Unified Search
	searchMsg := api.Message{
		ID:      "omni-query-1",
		Type:    api.TypeRequest,
		Sender:  "test-client",
		Target:  "omni-palette",
		Method:  "omni:search",
		Payload: []byte(`{"query": "omni", "limit": 10}`),
	}

	respChan := make(chan api.Message, 8)
	frontendID := "test-client"
	k.RegisterFrontend(frontendID, respChan)

	waitForResponse := func(expectedID string) api.Message {
		deadline := time.After(5 * time.Second)
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
				t.Fatalf("timeout waiting for %s response (last msg id=%s target=%s)", expectedID, lastMsg.ID, lastMsg.Target)
			}
		}
	}

	// Test Index directly first
	idxSearchMsg := api.Message{
		ID:      "idx-query-1",
		Type:    api.TypeRequest,
		Sender:  "test-client",
		Target:  "index",
		Method:  "knowledge:search",
		Payload: []byte(`{"query": "omni", "limit": 10}`),
	}
	k.RouteMessage(context.Background(), idxSearchMsg)

	respIdx := waitForResponse(idxSearchMsg.ID)
	fmt.Printf("Index search response (id=%s): %s\n", respIdx.ID, string(respIdx.Payload))

	k.RouteMessage(context.Background(), searchMsg)

	resp := waitForResponse(searchMsg.ID)

	fmt.Printf("Omni raw response (id=%s): %s\n", resp.ID, string(resp.Payload))

	if strings.Contains(string(resp.Payload), "error") {
		t.Fatalf("search returned error: %s", string(resp.Payload))
	}

	var results []struct {
		ID    string  `json:"id"`
		Title string  `json:"title"`
		Type  string  `json:"type"`
		Score float64 `json:"score"`
	}

	decodeResults := func(data []byte) error {
		if err := json.Unmarshal(data, &results); err != nil {
			var wrapper map[string]json.RawMessage
			if err2 := json.Unmarshal(data, &wrapper); err2 != nil {
				return err
			}
			inner, ok := wrapper["results"]
			if !ok {
				return err
			}
			if err := json.Unmarshal(inner, &results); err != nil {
				return err
			}
		}
		return nil
	}

	if err := decodeResults(resp.Payload); err != nil {
		t.Fatalf("failed to unmarshal results: %v payload=%s", err, string(resp.Payload))
	}

	// Verify we got results

	// Verify we got results
	fmt.Printf("Omni results found: %d\n", len(results))
	for _, res := range results {
		fmt.Printf("[%s] %s (id: %s, score: %.2f)\n", res.Type, res.Title, res.ID, res.Score)
	}

	if len(results) < 2 {
		t.Errorf("expected at least 2 results (command + doc), got %d", len(results))
	}

	foundDoc := false
	foundCmd := false
	for _, res := range results {
		if res.Type == "document" && res.ID == "doc-omni-test" {
			foundDoc = true
		}
		// In the real kernel, many commands match 'omni'
		if strings.Contains(strings.ToLower(res.ID), "omni") {
			foundCmd = true
		}
	}

	if !foundDoc {
		t.Error("doc-omni-test result was not found in omni-palette output")
	}
	if !foundCmd {
		t.Error("omni-related command was not found in omni-palette output")
	}
}
