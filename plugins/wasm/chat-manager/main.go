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
}

func main() {}

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
		eventReq := struct {
			Topic string      `json:"topic"`
			Data  ChatMessage `json:"data"`
		}{
			Topic: "chat:message",
			Data:  chatMsg,
		}
		eventPayload, _ := json.Marshal(eventReq)
		wasm.RouteMessage(wasm.Message{
			ID:        fmt.Sprintf("evt-chat-%d", chatMsg.Timestamp),
			Type:      "request",
			Sender:    "plugin-chat",
			Target:    "plugin-events",
			Method:    "publish",
			Payload:   eventPayload,
			Timestamp: chatMsg.Timestamp,
		})
		
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
