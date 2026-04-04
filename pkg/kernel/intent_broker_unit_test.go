package kernel

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/james-nesbitt/alloy/api"
)

func TestIntentBroker_ContextInjection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Mock router to capture the message
	var capturedMsg api.Message
	router := func(ctx context.Context, msg api.Message) {
		capturedMsg = msg
	}

	// Mock librarian querier
	querier := func(ctx context.Context, query string) (string, error) {
		if query == "test-query" {
			return "relevant-context-content", nil
		}
		return "", nil
	}

	broker := NewIntentBroker(logger, router, querier)
	broker.Register("test-plugin", []string{"ai:query"})

	// Dispatch an intent that should trigger context injection
	payload, _ := json.Marshal(map[string]string{"prompt": "test-query"})
	intent := api.Intent{
		ID:      "test-intent",
		Name:    "ai:query",
		Sender:  "user",
		Payload: payload,
	}

	err := broker.Dispatch(context.Background(), intent)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Verify the context was injected
	if capturedMsg.Metadata == nil {
		t.Fatal("Metadata is nil, context not injected")
	}

	contextValue, ok := capturedMsg.Metadata["semantic_context"].(string)
	if !ok || contextValue != "relevant-context-content" {
		t.Fatalf("Expected injected context 'relevant-context-content', got '%v'", capturedMsg.Metadata["semantic_context"])
	}
}

func TestIntentBroker_NoInjectionForNonAI(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Mock router
	var capturedMsg api.Message
	router := func(ctx context.Context, msg api.Message) {
		capturedMsg = msg
	}

	// Mock librarian querier
	querier := func(ctx context.Context, query string) (string, error) {
		return "should-not-be-injected", nil
	}

	broker := NewIntentBroker(logger, router, querier)
	broker.Register("test-plugin", []string{"other:intent"})

	intent := api.Intent{
		ID:      "test-intent",
		Name:    "other:intent",
		Sender:  "user",
		Payload: []byte(`{"prompt": "test-query"}`),
	}

	err := broker.Dispatch(context.Background(), intent)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Verify no context was injected
	if capturedMsg.Metadata != nil {
		if _, ok := capturedMsg.Metadata["semantic_context"]; ok {
			t.Fatal("Semantic context should not be injected for non-AI intents")
		}
	}
}
