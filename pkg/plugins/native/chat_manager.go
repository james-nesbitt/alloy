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

type ChatMessage struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type ChatManager struct {
	logger *slog.Logger
	state  storage.StateStore
	mu     sync.RWMutex
	router func(ctx context.Context, msg api.Message)
	
	// channelHistories stores recent messages for fast access
	channelHistories map[string][]ChatMessage
}

func NewChatManager(logger *slog.Logger, state storage.StateStore) *ChatManager {
	return &ChatManager{
		logger:           logger,
		state:            state,
		channelHistories: make(map[string][]ChatMessage),
	}
}

func (c *ChatManager) SetRouter(router func(ctx context.Context, msg api.Message)) {
	c.router = router
}

func (c *ChatManager) ID() string { return "plugin-chat" }

func (c *ChatManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "send", Description: "Send a message to a channel"},
		{Method: "history", Description: "Get message history for a channel"},
		{Method: "join", Description: "Join a chat channel"},
	}
}

func (c *ChatManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "send":
		var chatMsg ChatMessage
		if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
			return api.Message{}, err
		}

		chatMsg.Sender = msg.Sender
		chatMsg.Timestamp = time.Now().Unix()
		chatMsg.ID = fmt.Sprintf("msg-%d", time.Now().UnixNano())

		c.mu.Lock()
		c.channelHistories[chatMsg.Channel] = append(c.channelHistories[chatMsg.Channel], chatMsg)
		// Limit history in memory
		if len(c.channelHistories[chatMsg.Channel]) > 100 {
			c.channelHistories[chatMsg.Channel] = c.channelHistories[chatMsg.Channel][1:]
		}
		c.mu.Unlock()

		// Persist (naive approach: append to a list in state)
		// In a real app, we'd use a more structured log or doc store.
		go c.persistMessage(chatMsg)

		// Notify via events
		if c.router != nil {
			eventPayload, _ := json.Marshal(map[string]any{
				"topic": "chat:message",
				"data":  chatMsg,
			})
			c.router(ctx, api.Message{
				ID:        chatMsg.ID + "-evt",
				Type:      api.TypeEvent,
				Sender:    c.ID(),
				Target:    "plugin-events",
				Method:    "publish",
				Payload:   eventPayload,
				Timestamp: time.Now().Unix(),
			})
		}

		payload, _ := json.Marshal(chatMsg)
		return c.response(msg, payload), nil

	case "history":
		var req struct {
			Channel string `json:"channel"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		c.mu.RLock()
		history := c.channelHistories[req.Channel]
		c.mu.RUnlock()

		// If empty, try loading from state? 
		// For now we'll just return what's in memory.

		payload, _ := json.Marshal(history)
		return c.response(msg, payload), nil

	case "join":
		// Join currently just returns success and maybe current history
		var req struct {
			Channel string `json:"channel"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return api.Message{}, err
		}

		c.mu.RLock()
		history := c.channelHistories[req.Channel]
		c.mu.RUnlock()
		
		payload, _ := json.Marshal(map[string]any{
			"status": "joined",
			"history": history,
		})
		return c.response(msg, payload), nil
	}

	return api.Message{}, fmt.Errorf("unknown method: %s", msg.Method)
}

func (c *ChatManager) persistMessage(msg ChatMessage) {
	// For simplicity, we store history as a JSON list per channel
	c.mu.Lock()
	history := c.channelHistories[msg.Channel]
	c.mu.Unlock()

	data, _ := json.Marshal(history)
	_ = c.state.Set(c.ID(), "history:"+msg.Channel, data)
}

func (c *ChatManager) response(msg api.Message, payload []byte) api.Message {
	return api.Message{
		ID:        msg.ID + "-resp",
		Type:      api.TypeResponse,
		Sender:    c.ID(),
		Target:    msg.Sender,
		Method:    msg.Method,
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}
}

func (c *ChatManager) Shutdown(ctx context.Context) error {
	return nil
}
