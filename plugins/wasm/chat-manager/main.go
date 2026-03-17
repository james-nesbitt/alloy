package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

// Ensure symbols are NOT optimized away by referencing them in a way the compiler sees.
// We use a global variable and a function that the compiler cannot prove is unused.
var _ = wasm.Malloc

type ChatMessage struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

func main() {
	wasm.SetHandler(handleMessage)
}

// Ensure the binary doesn't exit and exports are available
//go:export malloc
func malloc(size uint32) uintptr {
	return wasm.Malloc(size)
}

func handleMessage(msg wasm.Message) wasm.Message {
	switch msg.Method {
	case "send":
		var chatMsg ChatMessage
		if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
			return errorResponse(msg, "failed to unmarshal chat message")
		}

		chatMsg.Sender = msg.Sender
		chatMsg.Timestamp = time.Now().Unix()
		chatMsg.ID = fmt.Sprintf("msg-%d", time.Now().UnixNano())

		// Persist using host KV
		// In WASM, we use the host's KV via the SDK
		historyKey := "history:" + chatMsg.Channel
		historyBytes := wasm.KVGet(historyKey)
		var history []ChatMessage
		if historyBytes != nil {
			json.Unmarshal(historyBytes, &history)
		}
		history = append(history, chatMsg)
		if len(history) > 100 {
			history = history[1:]
		}
		newHistoryBytes, _ := json.Marshal(history)
		wasm.KVSet(historyKey, newHistoryBytes)

		// Create a synthetic event message (to be routed by kernel)
		// Actually, in our architecture, the plugin returns a message to the kernel.
		// If the kernel sees Target="plugin-events", it will route it.
		// However, we want to return a response to the sender AND publish an event.
		// Since we only return one message, we "cheat" by returning a response 
		// but the SDK/Kernel should support returning multiple or we might need 
		// a separate 'publish' call in the SDK.
		
		// For now, let's just return the response. To publish an event, 
		// the plugin-chat would need to call a host function for 'RouteMessage'.
		
		payload, _ := json.Marshal(chatMsg)
		return wasm.Message{
			ID:        msg.ID + "-resp",
			Type:      "response",
			Sender:    "plugin-chat",
			Target:    msg.Sender,
			Method:    msg.Method,
			Payload:   payload,
			Timestamp: time.Now().Unix(),
		}

	case "ping":
		return wasm.Message{
			ID:        msg.ID + "-resp",
			Type:      "response",
			Sender:    "plugin-chat",
			Target:    msg.Sender,
			Method:    "ping",
			Payload:   []byte(`{"status":"pong"}`),
			Timestamp: time.Now().Unix(),
		}

	case "history":
		var req struct {
			Channel string `json:"channel"`
		}
		json.Unmarshal(msg.Payload, &req)

		historyBytes := wasm.KVGet("history:" + req.Channel)
		if historyBytes == nil {
			historyBytes = []byte("[]")
		}

		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-chat",
			Target:  msg.Sender,
			Method:  msg.Method,
			Payload: historyBytes,
		}

	default:
		return wasm.Message{}
	}
}

func errorResponse(msg wasm.Message, err string) wasm.Message {
	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-chat",
		Target: msg.Sender,
		Method: msg.Method,
		Payload: []byte(fmt.Sprintf(`{"error":"%s"}`, err)),
	}
}
