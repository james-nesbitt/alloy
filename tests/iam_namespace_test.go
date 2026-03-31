package tests

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/kernel"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

func TestIAMNamespaceRBAC(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	state := storage.NewMemoryStateStore()
	k, _ := kernel.New(logger, state, "", "")

	received := make(chan api.Message, 10)
	target := &mockTargetPlugin{received: received}
	k.RegisterPlugin(target)

	// Admin context for setup
	adminCtx := context.WithValue(context.Background(), "alloy.no_audit", true)

	// 1. Setup a role "project-contributor" with namespaced permissions
	// It can read buffers but ONLY in "prj-123" namespace
	policyPayload, _ := json.Marshal(map[string]any{
		"policy": map[string]any{
			"role": "project-contributor",
			"permissions": []string{
				"prj-123/target-plugin:read",
				"prj-123/target-plugin:write",
			},
		},
	})
	k.RouteMessage(adminCtx, api.Message{
		Sender:  "kernel",
		Target:  "iam",
		Method:  "policy:set",
		Payload: policyPayload,
	})

	// 2. Assign the role to a user
	identPayload, _ := json.Marshal(map[string]any{
		"actor": "user-a",
		"role":  "project-contributor",
	})
	k.RouteMessage(adminCtx, api.Message{
		Sender:  "kernel",
		Target:  "iam",
		Method:  "identity:set",
		Payload: identPayload,
	})

	time.Sleep(100 * time.Millisecond)

	t.Run("MatchCorrectNamespace", func(t *testing.T) {
		k.RouteMessage(context.Background(), api.Message{
			ID:     "msg-match",
			Sender: "user-a",
			Actor:  "user-a",
			Target: "target-plugin",
			Method: "read",
			Metadata: map[string]any{
				"context": "prj-123",
			},
		})

		select {
		case <-received:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("Access should have been granted for correct namespace")
		}
	})

	t.Run("DenyWrongNamespace", func(t *testing.T) {
		k.RouteMessage(context.Background(), api.Message{
			ID:     "msg-wrong-ns",
			Sender: "user-a",
			Actor:  "user-a",
			Target: "target-plugin",
			Method: "read",
			Metadata: map[string]any{
				"context": "prj-456",
			},
		})

		select {
		case <-received:
			t.Fatal("Access should have been denied for wrong namespace")
		case <-time.After(500 * time.Millisecond):
			// Success (blocked)
		}
	})

	t.Run("EphemeralGrantNamespace", func(t *testing.T) {
		// Grant user-a an ephemeral role in prj-456
		grantPayload, _ := json.Marshal(map[string]any{
			"actor":     "user-a",
			"namespace": "prj-456",
			"capabilities": []string{
				"target-plugin:read",
			},
		})
		k.RouteMessage(adminCtx, api.Message{
			Sender:  "kernel",
			Target:  "iam",
			Method:  "grant_namespace_role",
			Payload: grantPayload,
		})

		time.Sleep(100 * time.Millisecond)

		k.RouteMessage(context.Background(), api.Message{
			ID:     "msg-ephemeral",
			Sender: "user-a",
			Actor:  "user-a",
			Target: "target-plugin",
			Method: "read",
			Metadata: map[string]any{
				"context": "prj-456",
			},
		})

		select {
		case <-received:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("Access should have been granted via ephemeral grant")
		}
	})

	t.Run("GlobalWildcardNamespace", func(t *testing.T) {
		// Add a wildcard namespace permission to a new role
		policyPayload, _ := json.Marshal(map[string]any{
			"policy": map[string]any{
				"role": "global-viewer",
				"permissions": []string{
					"*/target-plugin:view",
				},
			},
		})
		k.RouteMessage(adminCtx, api.Message{
			Sender:  "kernel",
			Target:  "iam",
			Method:  "policy:set",
			Payload: policyPayload,
		})

		identPayload, _ := json.Marshal(map[string]any{
			"actor": "user-b",
			"role":  "global-viewer",
		})
		k.RouteMessage(adminCtx, api.Message{
			Sender:  "kernel",
			Target:  "iam",
			Method:  "identity:set",
			Payload: identPayload,
		})

		time.Sleep(100 * time.Millisecond)

		// Try different namespaces
		namespaces := []string{"prj-abc", "prj-xyz", "some-other-context"}
		for _, ns := range namespaces {
			k.RouteMessage(context.Background(), api.Message{
				ID:     "msg-wildcard-" + ns,
				Sender: "user-b",
				Actor:  "user-b",
				Target: "target-plugin",
				Method: "view",
				Metadata: map[string]any{
					"context": ns,
				},
			})

			select {
			case <-received:
				// Success
			case <-time.After(500 * time.Millisecond):
				t.Fatalf("Access failed for wildcard namespace: %s", ns)
			}
		}
	})
}

type mockTargetPlugin struct {
	received chan api.Message
}

func (p *mockTargetPlugin) ID() string                         { return "target-plugin" }
func (p *mockTargetPlugin) Capabilities() []api.Capability     { return nil }
func (p *mockTargetPlugin) Shutdown(ctx context.Context) error { return nil }
func (p *mockTargetPlugin) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	p.received <- msg
	return api.Message{ID: msg.ID + "-resp", Type: api.TypeResponse}, nil
}
