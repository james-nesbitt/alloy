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

func TestIAMInterceptor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	state := storage.NewMemoryStateStore()
	k, _ := kernel.New(logger, state, "", "")

	// 2. Load a target plugin
	target := &targetPlugin{received: make(chan api.Message, 1)}
	k.RegisterPlugin(target)

	// Context that skips auditing but NOT interception
	noAuditCtx := context.WithValue(context.Background(), "alloy.no_audit", true)

	t.Run("DefaultAllow", func(t *testing.T) {
		// Allow random-sender to talk to target-plugin
		payload, _ := json.Marshal(map[string]string{
			"sender": "random-sender",
			"target": "target-plugin",
		})
		k.RouteMessage(noAuditCtx, api.Message{
			Sender:  "kernel", // Kernel is admin, can set policies
			Method:  "allow",
			Target:  "iam",
			Payload: payload,
		})
		time.Sleep(50 * time.Millisecond)

		k.RouteMessage(noAuditCtx, api.Message{
			ID:     "msg-allow",
			Sender: "random-sender",
			Target: "target-plugin",
			Method: "hello",
		})
		select {
		case <-target.received:
			// Success
		case <-time.After(500 * time.Millisecond):
			t.Fatal("message should have been allowed and delivered")
		}
	})

	t.Run("PolicyDeny", func(t *testing.T) {
		// Define a policy that allows "restricted-user" ONLY to talk to "kernel"
		payload, _ := json.Marshal(map[string]string{
			"sender": "restricted-user",
			"target": "kernel",
		})
		k.RouteMessage(noAuditCtx, api.Message{
			Sender:  "kernel", // Admin
			Method:  "allow",
			Target:  "iam",
			Payload: payload,
		})

		// Give a tiny moment for IAM to process the internal message
		time.Sleep(50 * time.Millisecond)

		// Now attempt to talk to "target-plugin"
		k.RouteMessage(noAuditCtx, api.Message{
			ID:     "msg-deny",
			Sender: "restricted-user",
			Target: "target-plugin",
			Method: "hello",
		})

		select {
		case <-target.received:
			t.Fatal("message should have been denied by IAM policy")
		case <-time.After(500 * time.Millisecond):
			// Correct, was blocked
		}
	})

	t.Run("PolicyAllowOverride", func(t *testing.T) {
		// Update policy to allow target-plugin
		payload, _ := json.Marshal(map[string]string{
			"sender": "restricted-user",
			"target": "target-plugin",
		})
		k.RouteMessage(noAuditCtx, api.Message{
			Sender:  "kernel", // Admin
			Method:  "allow",
			Target:  "iam",
			Payload: payload,
		})

		// Give a tiny moment for IAM to process
		time.Sleep(50 * time.Millisecond)

		k.RouteMessage(noAuditCtx, api.Message{
			ID:     "msg-allow-2",
			Sender: "restricted-user",
			Target: "target-plugin",
			Method: "hello",
		})

		select {
		case <-target.received:
			// Success
		case <-time.After(500 * time.Millisecond):
			t.Fatal("message should have been allowed after policy update")
		}
	})
}
