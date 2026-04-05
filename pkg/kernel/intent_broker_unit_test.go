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

func TestIntentBroker_DispatchTarget(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var capturedMsg api.Message
	router := func(ctx context.Context, msg api.Message) {
		capturedMsg = msg
	}

	broker := NewIntentBroker(logger, router, nil)
	broker.Register("default-plugin", []string{"test:intent"})

	intent := api.Intent{
		ID:     "test-intent",
		Name:   "test:intent",
		Sender: "user",
		Target: "actor:claudine",
	}

	err := broker.Dispatch(context.Background(), intent)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if capturedMsg.Target != "actor:claudine" {
		t.Fatalf("Expected target 'actor:claudine', got '%s'", capturedMsg.Target)
	}
}

func TestIntentBroker_IntentDelegate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var capturedMsg api.Message
	router := func(ctx context.Context, msg api.Message) {
		capturedMsg = msg
	}

	broker := NewIntentBroker(logger, router, nil)
	broker.Register("actor:claudine", []string{"intent:delegate"})

	delegation := api.Delegation{
		ID:       "task-123",
		Owner:    "user",
		Assignee: "actor:claudine",
		Task:     "Implement Phase 12",
	}
	payload, _ := json.Marshal(delegation)

	intent := api.Intent{
		ID:      "intent-456",
		Name:    "intent:delegate",
		Sender:  "user",
		Target:  "actor:claudine",
		Payload: payload,
	}

	err := broker.Dispatch(context.Background(), intent)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if capturedMsg.Method != "intent:delegate" {
		t.Fatalf("Expected method 'intent:delegate', got '%s'", capturedMsg.Method)
	}

	var checkedDel api.Delegation
	if err := json.Unmarshal(capturedMsg.Payload, &checkedDel); err != nil {
		t.Fatalf("Failed to unmarshal delegation from payload: %v", err)
	}

	if checkedDel.ID != "task-123" || checkedDel.Assignee != "actor:claudine" {
		t.Fatalf("Delegation corrupted in dispatch: %+v", checkedDel)
	}
}

func TestIntentBroker_ActorCollaboration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var capturedMsg api.Message
	router := func(ctx context.Context, msg api.Message) {
		capturedMsg = msg
	}

	broker := NewIntentBroker(logger, router, nil)
	// Register a plugin that actor:claudine provides
	broker.Register("actor:claudine", []string{"service:format"})

	// actor:auditor requests formatting from actor:claudine
	intent := api.Intent{
		ID:     "collab-1",
		Name:   "service:format",
		Sender: "actor:auditor",
		Target: "actor:claudine",
	}

	err := broker.Dispatch(context.Background(), intent)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if capturedMsg.Sender != "actor:auditor" || capturedMsg.Target != "actor:claudine" {
		t.Fatalf("Collaboration dispatch failed: %+v", capturedMsg)
	}
}

func TestIntentBroker_ProactiveSuggestions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Mock router to capture the message
	var capturedMsg api.Message
	router := func(ctx context.Context, msg api.Message) {
		capturedMsg = msg
	}

	broker := NewIntentBroker(logger, router, nil)

	// Dispatch a proactive intent (intent:propose)
	intent := api.Intent{
		ID:      "propose-1",
		Name:    "intent:propose",
		Sender:  "actor:claudine",
		Payload: []byte(`{"title":"Fix Bug","description":"..."}`),
	}

	// Should fallback to broadcast (*) because no provider is registered for it
	err := broker.Dispatch(context.Background(), intent)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if capturedMsg.Target != "*" {
		t.Errorf("Expected target '*', got '%s'", capturedMsg.Target)
	}

	// Verify it still works if a provider IS registered
	broker.Register("explicit-provider", []string{"intent:suggest"})
	intent2 := api.Intent{
		ID:      "suggest-1",
		Name:    "intent:suggest",
		Sender:  "actor:claudine",
		Payload: []byte(`{}`),
	}

	err = broker.Dispatch(context.Background(), intent2)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if capturedMsg.Target != "explicit-provider" {
		t.Errorf("Expected target 'explicit-provider', got '%s'", capturedMsg.Target)
	}
}
