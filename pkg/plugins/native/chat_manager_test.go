package native

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
)

func TestChatManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	state := storage.NewMemoryStateStore()
	cm := NewChatManager(logger, state)

	ctx := context.Background()

	// 1. Send a message
	sendReq, _ := json.Marshal(map[string]string{
		"channel": "general",
		"content": "Hello world!",
	})
	msg := api.Message{
		ID:      "m1",
		Sender:  "user1",
		Method:  "send",
		Payload: sendReq,
	}

	resp, err := cm.HandleMessage(ctx, msg)
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	var chatMsg ChatMessage
	if err := json.Unmarshal(resp.Payload, &chatMsg); err != nil {
		t.Fatalf("failed to unmarshal chat response: %v", err)
	}

	if chatMsg.Content != "Hello world!" || chatMsg.Sender != "user1" {
		t.Errorf("chat message mismatch: %+v", chatMsg)
	}

	// 2. Get history
	histReq, _ := json.Marshal(map[string]string{
		"channel": "general",
	})
	msg = api.Message{
		ID:      "m2",
		Sender:  "user2",
		Method:  "history",
		Payload: histReq,
	}

	resp, err = cm.HandleMessage(ctx, msg)
	if err != nil {
		t.Fatalf("failed to get history: %v", err)
	}

	var history []ChatMessage
	if err := json.Unmarshal(resp.Payload, &history); err != nil {
		t.Fatalf("failed to unmarshal history: %v", err)
	}

	if len(history) != 1 || history[0].Content != "Hello world!" {
		t.Errorf("history mismatch: %+v", history)
	}

	// 3. Join channel
	joinReq, _ := json.Marshal(map[string]string{
		"channel": "general",
	})
	msg = api.Message{
		ID:      "m3",
		Sender:  "user3",
		Method:  "join",
		Payload: joinReq,
	}

	resp, err = cm.HandleMessage(ctx, msg)
	if err != nil {
		t.Fatalf("failed to join channel: %v", err)
	}

	var joinResult struct {
		Status  string        `json:"status"`
		History []ChatMessage `json:"history"`
	}
	json.Unmarshal(resp.Payload, &joinResult)
	if joinResult.Status != "joined" || len(joinResult.History) != 1 {
		t.Errorf("join result mismatch: %+v", joinResult)
	}
}
