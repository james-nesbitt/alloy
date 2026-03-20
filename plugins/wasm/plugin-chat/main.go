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

type Presence struct {
	User      string `json:"user"`
	Status    string `json:"status"` // online, away, offline
	Timestamp int64  `json:"timestamp"`
}

type DirectMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

func init() {
	wasm.SetHandler(handleMessage)
	wasm.SetCapabilities([]wasm.Capability{
		{Method: "send", Description: "Send a message to a channel", Shortcut: "c s", Annotations: map[string]string{"group": "chat"}},
		{Method: "history", Description: "Retrieve chat history for a channel", Shortcut: "c h", Annotations: map[string]string{"group": "chat"}},
		{Method: "direct:send", Description: "Send a direct message", Shortcut: "c d", Annotations: map[string]string{"group": "chat"}},
		{Method: "direct:history", Description: "Get direct message history", Shortcut: "c D", Annotations: map[string]string{"group": "chat"}},
		{Method: "ping", Description: "Check plugin health"},
		{Method: "presence:update", Description: "Update user presence status", Shortcut: "c p", Annotations: map[string]string{"group": "chat"}},
		{Method: "presence:list", Description: "List online users", Shortcut: "c l", Annotations: map[string]string{"group": "chat"}},
	})
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
		publishEvent("chat:message", chatMsg)
		
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

	case "direct:send":
		var dm DirectMessage
		if err := json.Unmarshal(msg.Payload, &dm); err != nil {
			return errorResponse(msg, "failed to unmarshal DM message")
		}
		dm.From = msg.Sender
		dm.Timestamp = time.Now().Unix()
		dm.ID = fmt.Sprintf("dm-%d", time.Now().UnixNano())

		// History key for DMs (sorted combo of IDs)
		pairKey := "dm:" + dm.From + ":" + dm.To
		if dm.From > dm.To {
			pairKey = "dm:" + dm.To + ":" + dm.From
		}
		
		historyBytes := wasm.KVGet(pairKey)
		var history []DirectMessage
		if historyBytes != nil {
			json.Unmarshal(historyBytes, &history)
		}
		history = append(history, dm)
		if len(history) > 50 {
			history = history[1:]
		}

		newHistoryBytes, _ := json.Marshal(history)
		wasm.KVSet(pairKey, newHistoryBytes)

		publishEvent("chat:direct", dm)

		payload, _ := json.Marshal(dm)
		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-chat",
			Target:  msg.Sender,
			Payload: payload,
		}

	case "presence:update":
		var presence Presence
		if err := json.Unmarshal(msg.Payload, &presence); err != nil {
			return errorResponse(msg, "invalid presence payload")
		}
		presence.User = msg.Sender
		presence.Timestamp = time.Now().Unix()

		// Update global presence list
		presenceListBytes := wasm.KVGet("presence:list")
		var presenceList map[string]Presence
		if presenceListBytes != nil {
			json.Unmarshal(presenceListBytes, &presenceList)
		} else {
			presenceList = make(map[string]Presence)
		}
		presenceList[msg.Sender] = presence
		
		// Prune old presence info
		for u, p := range presenceList {
			if time.Now().Unix()-p.Timestamp > 300 { // 5 minutes heartbeat
				delete(presenceList, u)
			}
		}

		newPresenceListBytes, _ := json.Marshal(presenceList)
		wasm.KVSet("presence:list", newPresenceListBytes)

		publishEvent("chat:presence", presence)

		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-chat",
			Target:  msg.Sender,
			Payload: []byte(`{"status":"updated"}`),
		}

	case "presence:list":
		presenceListBytes := wasm.KVGet("presence:list")
		if presenceListBytes == nil {
			presenceListBytes = []byte("{}")
		}
		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-chat",
			Target:  msg.Sender,
			Payload: presenceListBytes,
		}

	case "direct:history":
		var req struct {
			To string `json:"to"`
		}
		json.Unmarshal(msg.Payload, &req)

		pairKey := "dm:" + msg.Sender + ":" + req.To
		if msg.Sender > req.To {
			pairKey = "dm:" + req.To + ":" + msg.Sender
		}

		historyBytes := wasm.KVGet(pairKey)
		if historyBytes == nil {
			historyBytes = []byte("[]")
		}

		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-chat",
			Target:  msg.Sender,
			Payload: historyBytes,
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

func publishEvent(topic string, data any) {
	payload, _ := json.Marshal(struct {
		Topic string `json:"topic"`
		Data  any    `json:"data"`
	}{
		Topic: topic,
		Data:  data,
	})
	wasm.RouteMessage(wasm.Message{
		ID:        fmt.Sprintf("evt-%s-%d", topic, time.Now().UnixNano()),
		Type:      "request",
		Sender:    "plugin-chat",
		Target:    "plugin-events",
		Method:    "publish",
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	})
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
