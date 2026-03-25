package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

func TestUnifiedSanity(t *testing.T) {
	// Setup a clean runtime environment
	user := os.Getenv("USER")
	runtimeDir := filepath.Join("/tmp", fmt.Sprintf("alloy-sanity-%s", user))
	os.RemoveAll(runtimeDir)
	os.MkdirAll(runtimeDir, 0755)

	socket := filepath.Join(runtimeDir, "kernel.sock")

	// Get project root (assuming we are in tests/ directory)
	cwd, _ := os.Getwd()
	root := filepath.Dir(cwd)

	// Create a temporary provision file
	provisionPath := filepath.Join(runtimeDir, "provision.json")
	provision := map[string]interface{}{
		"plugins": []map[string]interface{}{
			{
				"id":        "project",
				"type":      "wasm",
				"path":      filepath.Join(root, "build/dist/usr/lib/alloy/plugins/project.wasm"),
				"load_time": "boot",
			},
			{
				"id":        "events",
				"type":      "native",
				"load_time": "boot",
			},
		},
	}
	provBytes, _ := json.Marshal(provision)
	os.WriteFile(provisionPath, provBytes, 0644)

	// Start Kernel in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	os.Setenv("ALLOY_TEST_MODE", "true")
	kernelCmd := exec.CommandContext(ctx, "go", "run", filepath.Join(root, "cmd/alloy-core"),
		"--listen", "unix://"+socket,
		"--insecure",
		"--provision", provisionPath,
		"--data-dir", filepath.Join(runtimeDir, "data"))

	kernelCmd.Stdout = os.Stdout
	kernelCmd.Stderr = os.Stderr

	if err := kernelCmd.Start(); err != nil {
		t.Fatalf("Failed to start kernel: %v", err)
	}

	// Wait for socket to appear
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if _, err := os.Stat(socket); err != nil {
		t.Fatal("Kernel failed to create socket in time")
	}

	// Create three clients (Simulating TUI, GUI, and Web Host)
	clients := make(map[string]*frontend.Client)
	for _, name := range []string{"tui-sim", "gui-sim", "web-host-sim"} {
		c, err := frontend.NewClient(name, socket, true)
		if err != nil {
			t.Fatalf("Failed to create client %s: %v", name, err)
		}
		clients[name] = c
		defer c.Close()
	}

	// Wait for plugin discovery to complete
	time.Sleep(2 * time.Second)

	// Step 1: Verification of Universal Discovery
	// Ensure all frontends see the 'project' plugin and its structured params
	for name, client := range clients {
		t.Run(fmt.Sprintf("Discovery-Check-%s", name), func(t *testing.T) {
			resp, err := client.Send(context.Background(), "system", "discovery:list", nil)
			if err != nil {
				t.Fatalf("Failed to fetch commands from %s: %v", name, err)
			}

			var wrapper struct {
				Targets []api.Registration `json:"targets"`
			}
			if err := json.Unmarshal(resp.Payload, &wrapper); err != nil {
				t.Fatalf("Malformed discovery payload from %s: %v (data: %s)", name, err, string(resp.Payload))
			}

			foundProject := false
			for _, reg := range wrapper.Targets {
				if reg.ID == "project" {
					foundProject = true
					// Check for capability 'project:create'
					foundCreate := false
					for _, cap := range reg.Capabilities {
						if cap.Method == "project:create" {
							foundCreate = true
							// Check annotations (params)
							if p, ok := cap.Annotations["params"]; !ok || p != "name,description" {
								t.Errorf("Plugin %s: project:create is missing correct 'params' metadata", name)
							}
							if g, ok := cap.Annotations["group"]; !ok || g != "project" {
								t.Errorf("Plugin %s: project:create is missing 'group' metadata", name)
							}
						}
					}
					if !foundCreate {
						t.Errorf("Plugin %s: project:create capability not found", name)
					}
				}
			}
			if !foundProject {
				t.Errorf("Client %s failed to discover 'project' plugin", name)
			}
		})
	}

	// Step 2: Verification of Event Broadcasting
	// If one frontend sends a request or the kernel fires an event, all should hear it.
	eventChan := make(chan api.Message, 10)
	for name, client := range clients {
		n := name

		// Subscribe to sanity topic
		subPayload, _ := json.Marshal(map[string]string{"topic": "sanity-topic"})
		_, err := client.Send(context.Background(), "events", "subscribe", subPayload)
		if err != nil {
			t.Fatalf("Client %s failed to subscribe: %v", name, err)
		}

		client.OnMessage(func(msg api.Message) {
			if msg.Method == "sanity-topic" {
				fmt.Printf("[%s] Received broadcast event\n", n)
				eventChan <- msg
			}
		})
	}

	// Publish to sanity topic from one client
	pubPayload, _ := json.Marshal(map[string]any{
		"topic": "sanity-topic",
		"data":  map[string]string{"msg": "sanity-check"},
	})

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer sendCancel()
	_, sendErr := clients["tui-sim"].Send(sendCtx, "events", "publish", pubPayload)
	if sendErr != nil {
		t.Errorf("Failed to publish sanity event: %v", sendErr)
	}

	// Expect 3 broadcast messages (one for each client)
	receivedCount := 0
	timeout := time.After(5 * time.Second)
	for receivedCount < 3 {
		select {
		case <-eventChan:
			receivedCount++
		case <-timeout:
			t.Errorf("Event broadcast failed: only received %d of 3 expected messages", receivedCount)
			goto FINISHED
		}
	}

FINISHED:
	fmt.Println("Sanity Check: Universal Discovery and Event Pipe verified.")
	cancel()
	kernelCmd.Wait()
}
