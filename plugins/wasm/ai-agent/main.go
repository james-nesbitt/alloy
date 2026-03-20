package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

type ChatMessage struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

func init() {
	wasm.SetHandler(handleMessage)
	wasm.SetCapabilities([]wasm.Capability{
		{Method: "summarize", Description: "Summarize provided text Content", Shortcut: "a s", Annotations: map[string]string{"group": "ai"}},
		{Method: "chat:message", Description: "AI reactive response", Shortcut: "a c", Annotations: map[string]string{"group": "ai"}},
	})
}

func main() {
	wasm.SleepForever()
}

func handleMessage(msg wasm.Message) wasm.Message {
	switch msg.Method {
	case "chat:message":
		var chatMsg ChatMessage
		if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
			return wasm.Message{}
		}

		if chatMsg.Sender == "plugin-ai-agent" {
			return wasm.Message{}
		}

		// AI response logic
		if len(chatMsg.Content) > 3 && (chatMsg.Content[:3] == "AI:" || chatMsg.Content[:3] == "ai:") {
			responseContent := "WASM AI Agent: I processed your request: " + chatMsg.Content[3:]
			chatReq, _ := json.Marshal(map[string]string{
				"channel": chatMsg.Channel,
				"content": responseContent,
			})

			// Return the message to the kernel for routing
			return wasm.Message{
				ID:        fmt.Sprintf("ai-resp-%d", time.Now().UnixNano()),
				Type:      "request",
				Sender:    "plugin-ai-agent",
				Target:    "plugin-chat",
				Method:    "send",
				Payload:   chatReq,
				Timestamp: time.Now().Unix(),
			}
		}
		return wasm.Message{}

	case "summarize":
		payload, _ := json.Marshal(map[string]string{"summary": "WASM AI Summary Created"})
		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-ai-agent",
			Target:  msg.Sender,
			Payload: payload,
		}

	default:
		return wasm.Message{}
	}
}
