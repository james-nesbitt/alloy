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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamicCommandDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dataDir := t.TempDir()
	kv := storage.NewMemoryStateStore()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	k, err := kernel.New(logger, kv, dataDir, "")
	require.NoError(t, err)
	defer k.Shutdown(ctx)

	cwd, _ := os.Getwd()
	projectRoot := filepath.Dir(cwd)

	// Load AI plugin
	aiWasm, err := os.ReadFile(filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/ai.wasm"))
	require.NoError(t, err)
	err = k.RegisterWASMPlugin("ai", aiWasm, []api.Capability{}) // Caps will be registered via WIT init
	require.NoError(t, err)

	// Load Buffer plugin
	bufferWasm, err := os.ReadFile(filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/buffer.wasm"))
	require.NoError(t, err)
	err = k.RegisterWASMPlugin("buffer", bufferWasm, []api.Capability{})
	require.NoError(t, err)

	// Wait for plugins to initialize and register capabilities via WIT calls
	time.Sleep(5 * time.Second)

	// Set up frontend to receive response
	frontendCh := make(chan api.Message, 10)
	k.RegisterFrontend("test-user", frontendCh)

	// Query command-manager for discovery
	k.RouteMessage(ctx, api.Message{
		ID:      "test-discover",
		Type:    api.TypeRequest,
		Sender:  "test-user",
		Target:  "command-manager",
		Method:  "discover",
		Payload: []byte("{}"),
	})

	var resp api.Message
	select {
	case resp = <-frontendCh:
		// Got response
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for discovery response")
	}

	var discoveryResult struct {
		Targets []api.Registration `json:"targets"`
	}
	err = json.Unmarshal(resp.Payload, &discoveryResult)
	require.NoError(t, err)

	foundAI := false
	foundBuffer := false
	var aiCaps []api.Capability
	var bufferCaps []api.Capability

	for _, target := range discoveryResult.Targets {
		if target.ID == "ai" {
			foundAI = true
			aiCaps = target.Capabilities
		}
		if target.ID == "buffer" {
			foundBuffer = true
			bufferCaps = target.Capabilities
		}
	}

	assert.True(t, foundAI, "AI plugin should be registered")
	assert.True(t, foundBuffer, "Buffer plugin should be registered")

	// Verify specific capabilities and shortcuts we added
	hasQuery := false
	for _, cap := range aiCaps {
		if cap.Method == "ai:query" {
			hasQuery = true
			assert.Equal(t, "a q", cap.Shortcut, "AI query should have shortcut 'a q'")
		}
	}
	assert.True(t, hasQuery, "AI plugin should have 'ai:query' capability in CommandManager")

	hasList := false
	for _, cap := range bufferCaps {
		if cap.Method == "buffer:list" {
			hasList = true
			assert.Equal(t, "b l", cap.Shortcut, "Buffer list should have shortcut 'b l'")
		}
	}
	assert.True(t, hasList, "Buffer plugin should have 'buffer:list' capability in CommandManager")
}
