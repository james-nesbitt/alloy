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

var (
	presenceStore = wasm.NewKVStore[map[string]Presence]("chat:presence")
)

func main() {
	p := wasm.New("plugin-chat").
		WithCapability("send", "Send a message to a channel", "c s").
		WithCapability("history", "Retrieve chat history for a channel", "c h").
		WithCapability("direct:send", "Send a direct message", "c d").
		WithCapability("direct:history", "Get direct message history", "c D").
		WithCapability("presence:update", "Update user presence status", "c p").
		WithCapability("presence:list", "List online users", "c l")

	p.Handle("send", func(msg wasm.Message) wasm.Message {
		var chatMsg ChatMessage
		if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
			return wasm.ErrorReply(msg, "failed to unmarshal chat message")
		}

		chatMsg.Sender = msg.Sender
		chatMsg.Timestamp = time.Now().Unix()
		chatMsg.ID = fmt.Sprintf("msg-%d", time.Now().UnixNano())

		// Persist history (simplified for now using KVStore)
		historyKey := "history:" + chatMsg.Channel
		historyBytes := wasm.KVGet(historyKey)
		var history []ChatMessage
		if historyBytes != nil {
			_ = json.Unmarshal(historyBytes, &history)
		}
		history = append(history, chatMsg)
		if len(history) > 100 { history = history[1:] }
		newHistoryBytes, _ := json.Marshal(history)
		wasm.KVSet(historyKey, newHistoryBytes)

		// Create a synthetic event message
		p.Events.Emit("chat:message", chatMsg)
		
		return wasm.Reply(msg, chatMsg)
	})

	p.Handle("direct:send", func(msg wasm.Message) wasm.Message {
		var dm DirectMessage
		if err := json.Unmarshal(msg.Payload, &dm); err != nil {
			return wasm.ErrorReply(msg, "failed to unmarshal DM message")
		}
		dm.From = msg.Sender
		dm.Timestamp = time.Now().Unix()
		dm.ID = fmt.Sprintf("dm-%d", time.Now().UnixNano())

		pairKey := "dm:" + dm.From + ":" + dm.To
		if dm.From > dm.To { pairKey = "dm:" + dm.To + ":" + dm.From }
		
		historyBytes := wasm.KVGet(pairKey)
		var history []DirectMessage
		if historyBytes != nil { _ = json.Unmarshal(historyBytes, &history) }
		history = append(history, dm)
		if len(history) > 50 { history = history[1:] }
		newHistoryBytes, _ := json.Marshal(history)
		wasm.KVSet(pairKey, newHistoryBytes)

		p.Events.Emit("chat:direct", dm)
		return wasm.Reply(msg, dm)
	})

	p.Handle("presence:update", func(msg wasm.Message) wasm.Message {
		var presence Presence
		_ = json.Unmarshal(msg.Payload, &presence)
		presence.User = msg.Sender
		presence.Timestamp = time.Now().Unix()

		presenceList, err := presenceStore.Get("list")
		if err != nil { presenceList = make(map[string]Presence) }
		presenceList[msg.Sender] = presence
		
		for u, pr := range presenceList {
			if time.Now().Unix()-pr.Timestamp > 300 { delete(presenceList, u) }
		}

		_ = presenceStore.Set("list", presenceList)
		p.Events.Emit("chat:presence", presence)
		return wasm.Reply(msg, map[string]string{"status": "updated"})
	})

	p.Handle("presence:list", func(msg wasm.Message) wasm.Message {
		presenceList, _ := presenceStore.Get("list")
		return wasm.Reply(msg, presenceList)
	})

	p.Handle("direct:history", func(msg wasm.Message) wasm.Message {
		var req struct { To string `json:"to"` }
		_ = json.Unmarshal(msg.Payload, &req)

		pairKey := "dm:" + msg.Sender + ":" + req.To
		if msg.Sender > req.To { pairKey = "dm:" + req.To + ":" + msg.Sender }

		historyBytes := wasm.KVGet(pairKey)
		var history []DirectMessage
		if historyBytes != nil { _ = json.Unmarshal(historyBytes, &history) }
		return wasm.Reply(msg, history)
	})

	p.Handle("history", func(msg wasm.Message) wasm.Message {
		var req struct { Channel string `json:"channel"` }
		_ = json.Unmarshal(msg.Payload, &req)

		historyBytes := wasm.KVGet("history:" + req.Channel)
		var history []ChatMessage
		if historyBytes != nil { _ = json.Unmarshal(historyBytes, &history) }
		return wasm.Reply(msg, history)
	})

	p.Run()
}
