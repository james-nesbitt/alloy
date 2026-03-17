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

func main() {
	wasm.SetHandler(handleMessage)

	// NOTE: wasm.Route is currently not supported by the host runtime.
	// Subscription and event routing are handled by returning properly
	// formatted messages to the kernel.
}

// malloc is needed for the host to allocate memory in the guest
//go:export malloc
func malloc(size uint32) uintptr {
	return wasm.Malloc(size)
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

		if len(chatMsg.Content) > 3 && chatMsg.Content[:3] == "AI:" {
			responseContent := "I'm a WASM AI agent! You said: " + chatMsg.Content[3:]
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
		var req struct {
			Text     string `json:"text,omitempty"`
			BufferID string `json:"buffer_id,omitempty"`
		}
		json.Unmarshal(msg.Payload, &req)
		
		text := req.Text
		if req.BufferID != "" {
			text = "Content of buffer " + req.BufferID
		}
		
		summary := "WASM SUMMARY: " + text
		if len(text) > 20 {
			summary = "WASM SUMMARY: " + text[:20] + "... "
		}
		
		payload, _ := json.Marshal(map[string]string{"summary": summary})
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
