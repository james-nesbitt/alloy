package tests

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/kernel"
	"github.com/jnesbitt/alloy-go/pkg/plugins/native"
	"github.com/jnesbitt/alloy-go/pkg/storage"
)

func TestIAMInterceptor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	k := kernel.New(logger)
	state := storage.NewMemoryStateStore()

	// 1. Load IAM
	iam, _ := native.NewIdentityManager(ctx, logger, state)
	k.RegisterPlugin(iam.(api.Plugin))

	// 2. Load a target plugin
	target := &targetPlugin{received: make(chan api.Message, 1)}
	k.RegisterPlugin(target)

	// Context that skips auditing but NOT interception
	noAuditCtx := context.WithValue(context.Background(), "alloy.no_audit", true)

	t.Run("DefaultAllow", func(t *testing.T) {
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
			Method:  "allow",
			Target:  "plugin-iam",
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
			Method:  "allow",
			Target:  "plugin-iam",
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
