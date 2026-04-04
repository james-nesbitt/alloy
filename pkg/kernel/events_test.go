package kernel

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"log/slog"
	"os"
)

func TestEventManagerPatternSubscription(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	em := NewEventManager(logger, nil)

	received := make(chan api.Message, 1)
	em.SetRouter(func(ctx context.Context, msg api.Message) {
		received <- msg
	})

	// Subscribe with pattern
	subMsg := api.Message{
		ID:      "sub-1",
		Sender:  "test-plugin",
		Method:  "subscribe",
		Payload: []byte(`{"pattern":"log:.*"}`),
	}
	_, err := em.HandleMessage(context.Background(), subMsg)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish matching event
	testData := json.RawMessage(`{"msg":"hello"}`)
	em.Publish(context.Background(), "log:info", "sender", testData)

	select {
	case msg := <-received:
		if msg.Method != "log:info" {
			t.Errorf("Expected method log:info, got %s", msg.Method)
		}
	case <-time.After(1 * time.Second):
		t.Error("Did not receive event on pattern subscription")
	}

	// Publish non-matching event
	em.Publish(context.Background(), "other:topic", "sender", testData)
	select {
	case <-received:
		t.Error("Received event that should not match pattern")
	case <-time.After(100 * time.Millisecond):
		// Success
	}
}

func TestEventManagerTopicAndPatternDeduplication(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	em := NewEventManager(logger, nil)

	var count int
	em.SetRouter(func(ctx context.Context, msg api.Message) {
		count++
	})

	// Subscribe to topic and pattern matching that topic
	em.HandleMessage(context.Background(), api.Message{
		ID:      "sub-1",
		Sender:  "test-plugin",
		Method:  "subscribe",
		Payload: []byte(`{"topic":"log:info"}`),
	})
	em.HandleMessage(context.Background(), api.Message{
		ID:      "sub-2",
		Sender:  "test-plugin",
		Method:  "subscribe",
		Payload: []byte(`{"pattern":"log:.*"}`),
	})

	em.Publish(context.Background(), "log:info", "sender", []byte(`{}`))

	time.Sleep(100 * time.Millisecond)
	if count != 1 {
		t.Errorf("Expected 1 received message (deduplicated), got %d", count)
	}
}
