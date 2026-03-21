package tests

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/kernel"
)

type mockInterceptor struct {
	count atomic.Int32
	deny  bool
}

func (m *mockInterceptor) PreRoute(ctx context.Context, msg api.Message) (api.Message, bool, error) {
	m.count.Add(1)
	if m.deny {
		return msg, false, nil
	}
	// Add metadata to prove interception
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]any)
	}
	msg.Metadata["intercepted"] = true
	return msg, true, nil
}

// Implement Plugin interface so it can be registered
func (m *mockInterceptor) ID() string                        { return "mock-interceptor" }
func (m *mockInterceptor) Capabilities() []api.Capability   { return nil }
func (m *mockInterceptor) Shutdown(ctx context.Context) error { return nil }
func (m *mockInterceptor) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	return api.Message{}, nil
}

func TestInterceptors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	k := kernel.New(logger, "")

	interceptor := &mockInterceptor{}
	target := &targetPlugin{received: make(chan api.Message, 1)}

	k.RegisterPlugin(interceptor)
	k.RegisterPlugin(target)

	t.Run("AllowedRouting", func(t *testing.T) {
		interceptor.count.Store(0)
		msg := api.Message{
			ID:     "msg-1",
			Sender: "tester",
			Target: "target-plugin",
			Method: "hello",
		}

		k.RouteMessage(context.Background(), msg)

		received := <-target.received
		if received.Metadata["intercepted"] != true {
			t.Error("message Metadata was not modified by interceptor")
		}
		if interceptor.count.Load() != 1 {
			t.Errorf("interceptor was called %d times, expected 1", interceptor.count.Load())
		}
	})

	t.Run("DeniedRouting", func(t *testing.T) {
		interceptor.count.Store(0)
		interceptor.deny = true
		msg := api.Message{
			ID:     "msg-2",
			Sender: "tester",
			Target: "target-plugin",
			Method: "hello",
		}

		k.RouteMessage(context.Background(), msg)

		// Check that the target didn't receive it
		select {
		case <-target.received:
			t.Error("target received a message that should have been denied")
		default:
			// Success
		}

		if interceptor.count.Load() != 1 {
			t.Errorf("interceptor was called %d times, expected 1", interceptor.count.Load())
		}
	})
}
