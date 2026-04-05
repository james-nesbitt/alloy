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
	"github.com/james-nesbitt/alloy/pkg/project"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

func TestWorkspaceSynthesis(t *testing.T) {
	// 1. Setup
	tempDir, err := os.MkdirTemp("", "synthesis-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	kv, _ := storage.NewFileStateStore(filepath.Join(tempDir, "storage"))
	k, err := kernel.New(logger, kv, tempDir, "")
	if err != nil {
		t.Fatal(err)
	}
	k.SetInsecure(true)
	defer k.Shutdown(context.Background())

	cwd, _ := os.Getwd()
	projectRoot := cwd
	if filepath.Base(cwd) == "tests" {
		projectRoot = filepath.Dir(cwd)
	}

	// Build paths to plugins
	aiWasm := filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/ai.wasm")
	switcherWasm := filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/switcher.wasm")
	projectWasm := filepath.Join(projectRoot, "build/dist/usr/lib/alloy/plugins/project.wasm")

	// 2. Create User Config with a side-car (switcher)
	userConfig := &project.UserConfig{
		Sidecars: []project.PluginConfig{
			{ID: "switcher", Path: switcherWasm, LoadTime: api.LoadTimeBoot},
		},
	}
	userConfigPath := filepath.Join(tempDir, "user-config.json")
	userContent, _ := json.Marshal(userConfig)
	os.WriteFile(userConfigPath, userContent, 0644)

	// 3. Create Project Manifest for ProjA
	manifestA := &project.ProjectManifest{
		Name: "ProjA",
		Plugins: []project.PluginConfig{
			{ID: "ai", Path: aiWasm, LoadTime: api.LoadTimeBoot},
			{ID: "project", Path: projectWasm, LoadTime: api.LoadTimeBoot}, // Need project plugin to manage state
		},
	}
	manifestAPath := filepath.Join(tempDir, "alloy-project.json")
	manifestAContent, _ := json.Marshal(manifestA)
	os.WriteFile(manifestAPath, manifestAContent, 0644)

	// 4. Register Frontend to listen for events
	frontendCh := make(chan api.Message, 100)
	k.RegisterFrontend("test-user", frontendCh)

	// 5. Bootstrap - simulate what alloy-core main would do
	k.Provision([]kernel.PluginDef{
		{ID: "switcher", Path: switcherWasm, Type: "wasm", LoadTime: api.LoadTimeBoot},
		{ID: "project", Path: projectWasm, Type: "wasm", LoadTime: api.LoadTimeBoot},
		{ID: "ai", Path: aiWasm, Type: "wasm", LoadTime: api.LoadTimeBoot},
	})

	// Wait for boot and bootstrap messages
	// Wait for boot and bootstrap messages
	time.Sleep(10 * time.Second)

	// Simulate alloy-core pushing user config
	userContent, _ = json.Marshal(userConfig)
	k.Call(context.Background(), api.Message{
		ID:      "manual-user-config",
		Sender:  "system",
		Target:  "project",
		Method:  "project:update-user-config",
		Payload: userContent,
	})

	// Simulate alloy-core pushing manifest workspace
	wsData, _ := json.Marshal(map[string]interface{}{
		"id":     manifestA.Name,
		"name":   manifestA.Name,
		"path":   tempDir,
		"layout": manifestA.Layout,
	})
	k.Call(context.Background(), api.Message{
		ID:      "manual-manifest-import",
		Sender:  "system",
		Target:  "project",
		Method:  "project:import",
		Payload: wsData,
	})

	// Wait a bit
	time.Sleep(500 * time.Millisecond)

	// 6. Verify Discovery
	discoverMsg := api.Message{
		ID:     "probe-discovery",
		Sender: "test-user",
		Target: "command-manager",
		Method: "discover",
	}
	resp, err := k.Call(context.Background(), discoverMsg)
	if err != nil {
		t.Fatal(err)
	}

	var discovery struct {
		Targets []api.Registration `json:"targets"`
	}
	json.Unmarshal(resp.Payload, &discovery)
	t.Logf("Discovery targets: %v", discovery.Targets)

	foundSwitcher := false
	foundAI := false
	for _, t := range discovery.Targets {
		if t.ID == "switcher" {
			foundSwitcher = true
		}
		if t.ID == "ai" {
			foundAI = true
		}
	}

	if !foundSwitcher {
		t.Fatal("Switcher side-car not found in discovery")
	}
	if !foundAI {
		t.Fatal("Project AI plugin not found in discovery")
	}

	// 7. Test Composition Logic
	var activePlugins []api.PluginMetadata
	for _, m := range k.GetPluginMetadata() {
		activePlugins = append(activePlugins, m)
	}

	composed := project.Compose(manifestA, userConfig, activePlugins)

	if composed.Name != "ProjA" {
		t.Errorf("Expected composed name ProjA, got %s", composed.Name)
	}

	hasSwitcher := false
	hasAI := false
	for _, p := range composed.Plugins {
		if p.ID == "switcher" {
			hasSwitcher = true
		}
		if p.ID == "ai" {
			hasAI = true
		}
	}

	if !hasSwitcher || !hasAI {
		t.Errorf("Composed workspace missing expected plugins: switcher=%v, ai=%v", hasSwitcher, hasAI)
	}

	t.Log("Workspace Synthesis test passed!")
}
