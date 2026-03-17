package native

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
)

type AIManager struct {
	logger *slog.Logger
	state  storage.StateStore
	mu     sync.RWMutex
	router func(ctx context.Context, msg api.Message)
	
	enabled bool
}

func NewAIManager(logger *slog.Logger, state storage.StateStore) *AIManager {
	return &AIManager{
		logger: logger,
		state:  state,
	}
}

func (a *AIManager) SetRouter(router func(ctx context.Context, msg api.Message)) {
	a.router = router
}

func (a *AIManager) ID() string { return "plugin-ai-agent" }

func (a *AIManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "chat:message", Description: "Handle chat messages"},
		{Method: "summarize", Description: "Summarize a buffer or text"},
		{Method: "ask", Description: "General AI query"},
	}
}

func (a *AIManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "chat:message":
		// This is triggered via event subscription usually
		var chatMsg ChatMessage
		if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
			return api.Message{}, err
		}

		// Don't respond to ourselves
		if chatMsg.Sender == a.ID() {
			return api.Message{}, nil
		}

		a.logger.Info("AI agent received chat message", "content", chatMsg.Content)

		// Mock response logic: if it starts with "AI:", respond
		if len(chatMsg.Content) > 3 && chatMsg.Content[:3] == "AI:" {
			go a.respondToChat(ctx, chatMsg.Channel, "I'm a mock AI agent! You said: "+chatMsg.Content[3:])
		}
		return api.Message{}, nil

	case "summarize":
		var req struct {
			Text     string `json:"text,omitempty"`
			BufferID string `json:"buffer_id,omitempty"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		textToSummarize := req.Text
		if req.BufferID != "" {
			// Fetch from Buffer Manager
			getReq, _ := json.Marshal(map[string]string{"id": req.BufferID})
			// This is a synchronous request-response over the bus!
			// We need a helper for this or use a channel.
			// For simplicity in native code, we can just call and wait if we have a way.
			// But the kernel/bus is async.
			// Let's just mock it for now or implement a sync call in the manager.
			textToSummarize = "Content of buffer " + req.BufferID
		}

		summary := "SUMMARY (Mock): " + textToSummarize
		if len(req.Text) > 20 {
			summary = "SUMMARY (Mock): " + req.Text[:20] + "..."
		}

		payload, _ := json.Marshal(map[string]string{"summary": summary})
		return a.response(msg, payload), nil

	case "ask":
		var req struct {
			Query string `json:"query"`
		}
		json.Unmarshal(msg.Payload, &req)

		payload, _ := json.Marshal(map[string]string{"answer": "AI Answer to: " + req.Query})
		return a.response(msg, payload), nil
	}

	return api.Message{}, fmt.Errorf("unknown method: %s", msg.Method)
}

func (a *AIManager) respondToChat(ctx context.Context, channel, content string) {
	if a.router == nil {
		return
	}

	time.Sleep(500 * time.Millisecond) // Mock thinking time

	chatReq, _ := json.Marshal(map[string]string{
		"channel": channel,
		"content": content,
	})

	a.router(ctx, api.Message{
		ID:        fmt.Sprintf("ai-resp-%d", time.Now().UnixNano()),
		Type:      api.TypeRequest,
		Sender:    a.ID(),
		Target:    "plugin-chat",
		Method:    "send",
		Payload:   chatReq,
		Timestamp: time.Now().Unix(),
	})
}

func (a *AIManager) response(msg api.Message, payload []byte) api.Message {
	return api.Message{
		ID:        msg.ID + "-resp",
		Type:      api.TypeResponse,
		Sender:    a.ID(),
		Target:    msg.Sender,
		Method:    msg.Method,
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}
}

func (a *AIManager) Shutdown(ctx context.Context) error {
	return nil
}
