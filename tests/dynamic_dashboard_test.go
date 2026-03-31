package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/kernel"
	"github.com/james-nesbitt/alloy/pkg/project"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

type mockPlugin struct {
	id      string
	handler func(context.Context, api.Message) (api.Message, error)
}

func (m *mockPlugin) ID() string                     { return m.id }
func (m *mockPlugin) Capabilities() []api.Capability { return nil }
func (m *mockPlugin) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	return m.handler(ctx, msg)
}
func (m *mockPlugin) Shutdown(ctx context.Context) error { return nil }

func TestDynamicDashboardWidgets(t *testing.T) {
	dataDir := t.TempDir()
	kvStore := storage.NewMemoryStateStore()
	logH := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	k, err := kernel.New(logH, kvStore, dataDir, "")
	if err != nil {
		t.Fatalf("failed to create WIT kernel: %v", err)
	}

	// Register a mock dashboard plugin to receive events
	widgetCh := make(chan api.Widget, 10)
	k.RegisterPlugin(&mockPlugin{
		id: "dashboard-mock",
		handler: func(ctx context.Context, msg api.Message) (api.Message, error) {
			if msg.Method == "dashboard:widget-registered" {
				var w api.Widget
				if err := json.Unmarshal(msg.Payload, &w); err == nil {
					widgetCh <- w
				}
			}
			return api.Message{}, nil
		},
	})

	// Add subscription
	k.RouteMessage(context.Background(), api.Message{
		ID:      "sub-1",
		Sender:  "dashboard-mock",
		Target:  "events",
		Method:  "subscribe",
		Payload: []byte(`{"topic":"dashboard:widget-registered"}`),
	})

	// Load the AI plugin which registers a widget
	wasmBytes, err := os.ReadFile("../build/dist/usr/lib/alloy/plugins/ai.wasm")
	if err != nil {
		t.Skip("ai.wasm not found, run just build-plugins first")
	}

	err = k.RegisterWASMPlugin("ai", wasmBytes, []api.Capability{})
	if err != nil {
		t.Fatalf("failed to register AI plugin: %v", err)
	}

	// Wait for widget registration event
	select {
	case w := <-widgetCh:
		if w.ID != "ai-status" {
			t.Errorf("expected widget ID 'ai-status', got %s", w.ID)
		}
		if w.Title != "AI Assistant" {
			t.Errorf("expected widget title 'AI Assistant', got %s", w.Title)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for widget registration")
	}
}

func TestManifestAutoBoot(t *testing.T) {
	dataDir := t.TempDir()
	kvStore := storage.NewMemoryStateStore()
	logH := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	k, err := kernel.New(logH, kvStore, dataDir, "")
	if err != nil {
		t.Fatalf("failed to create kernel: %v", err)
	}

	manifestPath := filepath.Join(dataDir, "alloy-project.json")
	cwd, _ := os.Getwd()
	pluginPath := filepath.Join(filepath.Dir(cwd), "build/dist/usr/lib/alloy/plugins/ai.wasm")

	if _, err := os.Stat(pluginPath); err != nil {
		t.Skip("ai.wasm not found")
	}

	manifest := fmt.Sprintf(`{
		"name": "Test Project",
		"plugins": [
			{ "id": "ai", "path": "%s", "load": "boot" }
		]
	}`, pluginPath)

	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	m, err := project.LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}

	// Capture events
	widgetCh := make(chan struct{}, 10)
	k.RegisterPlugin(&mockPlugin{
		id: "event-catcher",
		handler: func(ctx context.Context, msg api.Message) (api.Message, error) {
			if msg.Method == "dashboard:widget-registered" {
				widgetCh <- struct{}{}
			}
			return api.Message{}, nil
		},
	})
	k.RouteMessage(context.Background(), api.Message{
		ID:      "sub-2",
		Sender:  "event-catcher",
		Target:  "events",
		Method:  "subscribe",
		Payload: []byte(`{"topic":"dashboard:widget-registered"}`),
	})

	// We no longer manually boot project things in core tests as core is agostic.
	// Instead, we just provision the plugins and see if they work.

	for _, pc := range m.Plugins {
		pluginPath := kernel.ResolvePluginPath(manifestPath, pc.Path)
		pDef := kernel.PluginDef{
			ID:       pc.ID,
			Path:     pluginPath,
			Type:     "wasm",
			LoadTime: pc.LoadTime,
		}
		if err := k.Provision([]kernel.PluginDef{pDef}); err != nil {
			t.Fatalf("failed to provision plugin %s: %v", pc.ID, err)
		}
	}

	// Verify plugin 'ai' is loading and eventually registers its widget
	select {
	case <-widgetCh:
		// Success
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for auto-booted widget")
	}
}
