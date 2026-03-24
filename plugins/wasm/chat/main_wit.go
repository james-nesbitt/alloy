//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/build/gen/bindings/guest"
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"time"
)

// ChatMessage represents a message in a channel.
type ChatMessage struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// Presence represents a user's presence status.
type Presence struct {
	User      string `json:"user"`
	Status    string `json:"status"` // online, away, offline
	Timestamp int64  `json:"timestamp"`
}

// DirectMessage represents a direct message between users.
type DirectMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

var plugin *Plugin

func main() {
	// Create a new WIT-based plugin
	plugin = NewPlugin("chat").
		WithMetadata(
			"Chat Plugin", 
			"Provides chat functionality including channels and direct messages",
			"0.1.0", 
			"Alloy Team",
		).
		WithTags("chat", "messaging", "communication").
		WithCapability("chat:send", "Send a message to a channel").
		WithCapability("chat:history", "Retrieve chat history for a channel").
		WithCapability("chat:direct:send", "Send a direct message").
		WithCapability("chat:direct:history", "Get direct message history").
		WithCapability("chat:presence:update", "Update user presence status").
		WithCapability("chat:presence:list", "List online users").
		WithCapability("ui:view:chat", "Open the chat view").
		WithAnnotations("ui:view:chat", map[string]string{"type": "chat", "title": "Team Chat"})

	// Set up message handlers
	plugin.Handle("chat:send", handleSendMessage)
	plugin.Handle("chat:direct:send", handleDirectSend)
	plugin.Handle("chat:presence:update", handlePresenceUpdate)
	plugin.Handle("chat:presence:list", handlePresenceList)
	plugin.Handle("chat:direct:history", handleDirectHistory)
	plugin.Handle("chat:history", handleHistory)
	plugin.Handle("ui:view:chat", func(msg AlloyMessage) AlloyMessage { return plugin.Reply(msg, "ok") })

	// Backward compatibility handlers
	plugin.Handle("send", handleSendMessage)
	plugin.Handle("direct:send", handleDirectSend)
	plugin.Handle("presence:update", handlePresenceUpdate)
	plugin.Handle("presence:list", handlePresenceList)
	plugin.Handle("direct:history", handleDirectHistory)
	plugin.Handle("history", handleHistory)

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "Chat plugin initializing")
		return nil
	})

	// Run the plugin
	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

// handleSendMessage handles sending a message to a channel.
func handleSendMessage(msg AlloyMessage) AlloyMessage {
	var chatMsg ChatMessage
	if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_chat_message")
	}

	chatMsg.Sender = msg.Sender
	chatMsg.Timestamp = time.Now().Unix()
	chatMsg.ID = fmt.Sprintf("msg-%d", time.Now().UnixNano())

	// Persist history
	historyKey := "history:" + chatMsg.Channel
	historyData, _ := plugin.KVGet(historyKey)
	var history []ChatMessage
	if historyData != nil {
		_ = json.Unmarshal(historyData, &history)
	}
	history = append(history, chatMsg)
	if len(history) > 100 {
		history = history[1:]
	}
	newHistoryData, _ := json.Marshal(history)
	plugin.KVSet(historyKey, newHistoryData)

	// Broadcast the message via events service
	evtPayload, _ := json.Marshal(map[string]interface{}{
		"topic": "chat:message",
		"data":  chatMsg,
	})
	plugin.RouteMessage(AlloyMessage{
		MsgType: "request",
		Method:  "publish",
		Sender:  "chat",
		Target:  Some("events"),
		Payload: evtPayload,
	})

	return plugin.Reply(msg, chatMsg)
}

// handleDirectSend handles sending a direct message.
func handleDirectSend(msg AlloyMessage) AlloyMessage {
	var dm DirectMessage
	if err := json.Unmarshal(msg.Payload, &dm); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_dm_message")
	}
	dm.From = msg.Sender
	dm.Timestamp = time.Now().Unix()
	dm.ID = fmt.Sprintf("dm-%d", time.Now().UnixNano())

	// Create a key for the message pair
	pairKey := "dm:" + dm.From + ":" + dm.To
	if dm.From > dm.To {
		pairKey = "dm:" + dm.To + ":" + dm.From
	}

	// Persist history
	historyData, _ := plugin.KVGet(pairKey)
	var history []DirectMessage
	if historyData != nil {
		_ = json.Unmarshal(historyData, &history)
	}
	history = append(history, dm)
	if len(history) > 50 {
		history = history[1:]
	}
	newHistoryData, _ := json.Marshal(history)
	plugin.KVSet(pairKey, newHistoryData)

	// Broadcast the direct message
	plugin.RouteMessage(AlloyMessage{
		MsgType: "event",
		Method:  "chat:direct",
		Sender:  "chat-plugin",
		Payload: newHistoryData,
	})

	return plugin.Reply(msg, dm)
}

// handlePresenceUpdate handles updating user presence.
func handlePresenceUpdate(msg AlloyMessage) AlloyMessage {
	var presence Presence
	if err := json.Unmarshal(msg.Payload, &presence); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_presence")
	}
	presence.User = msg.Sender
	presence.Timestamp = time.Now().Unix()

	// Get current presence list
	presenceData, _ := plugin.KVGet("chat:presence")
	var presenceList map[string]Presence
	if presenceData != nil {
		_ = json.Unmarshal(presenceData, &presenceList)
	} else {
		presenceList = make(map[string]Presence)
	}

	// Update presence
	presenceList[msg.Sender] = presence

	// Clean up stale presence
	for user, pr := range presenceList {
		if time.Now().Unix()-pr.Timestamp > 300 {
			delete(presenceList, user)
		}
	}

	// Save updated presence
	updatedData, _ := json.Marshal(presenceList)
	plugin.KVSet("chat:presence", updatedData)

	// Broadcast presence update
	plugin.RouteMessage(AlloyMessage{
		MsgType: "event",
		Method:  "chat:presence",
		Sender:  "chat-plugin",
		Payload: msg.Payload,
	})

	return plugin.Reply(msg, map[string]string{"status": "updated"})
}

// handlePresenceList handles listing online users.
func handlePresenceList(msg AlloyMessage) AlloyMessage {
	// Get current presence list
	presenceData, _ := plugin.KVGet("chat:presence")
	var presenceList map[string]Presence
	if presenceData != nil {
		_ = json.Unmarshal(presenceData, &presenceList)
	} else {
		presenceList = make(map[string]Presence)
	}

	// Clean up stale presence
	for user, pr := range presenceList {
		if time.Now().Unix()-pr.Timestamp > 300 {
			delete(presenceList, user)
		}
	}

	// Save updated presence
	updatedData, _ := json.Marshal(presenceList)
	plugin.KVSet("chat:presence", updatedData)

	return plugin.Reply(msg, presenceList)
}

// handleDirectHistory handles getting direct message history.
func handleDirectHistory(msg AlloyMessage) AlloyMessage {
	var req struct {
		To string `json:"to"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	// Create a key for the message pair
	pairKey := "dm:" + msg.Sender + ":" + req.To
	if msg.Sender > req.To {
		pairKey = "dm:" + req.To + ":" + msg.Sender
	}

	// Get history
	historyData, _ := plugin.KVGet(pairKey)
	var history []DirectMessage
	if historyData != nil {
		_ = json.Unmarshal(historyData, &history)
	}

	return plugin.Reply(msg, history)
}

// handleHistory handles getting channel history.
func handleHistory(msg AlloyMessage) AlloyMessage {
	var req struct {
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	// Get history
	historyData, _ := plugin.KVGet("history:" + req.Channel)
	var history []ChatMessage
	if historyData != nil {
		_ = json.Unmarshal(historyData, &history)
	}

	return plugin.Reply(msg, history)
}
