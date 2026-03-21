//go:build wasip1 || wasm

package main

import (
	"encoding/json"
	"fmt"
	"github.com/jnesbitt/alloy-go/pkg/wasm2/bindings/guest"
)

func main() {
	// Initialize the chat plugin
	pluginID := "wit-chat-plugin"
	caps := []guest.AlloyCapability{
		{
			Method:      "chat:send",
			Description: "Send a chat message",
		},
		{
			Method:      "chat:receive",
			Description: "Receive chat messages",
		},
		{
			Method:      "chat:history",
			Description: "Get chat history",
		},
	}

	// Initialize the plugin with the host
	guest.AlloyInit(pluginID, caps)

	// Signal that the plugin is ready
	guest.AlloyStarted()

	guest.AlloyLog("info", "Chat plugin initialized")

	// Set up chat history in KV storage
	err := setupChatHistory()
	if err != nil {
		guest.AlloyLog("error", "Failed to set up chat history: "+err.Error())
	}

	// Main message loop
	for {
		// Get the next message
		msgOpt := guest.AlloyGetNextMessage()
		if !msgOpt.Set {
			guest.AlloyYield()
			continue
		}

		msg := msgOpt.Value
		guest.AlloyLog("info", "Received message: "+msg.Method)

		// Handle the message
		var resp guest.AlloyMessage
		if msg.Method == "chat:send" {
			resp = handleSendMessage(msg)
		} else if msg.Method == "chat:history" {
			resp = handleHistoryRequest(msg)
		} else {
			errMsg := map[string]string{"error": "method_not_found"}
			errData, _ := json.Marshal(errMsg)
			resp = guest.AlloyMessage{
				Id:      msg.Id + "-response",
				Method:  msg.Method,
				Sender:  pluginID,
				Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
				Payload: errData,
			}
		}

		// Send the response
		guest.AlloySendResponse(resp)
	}
}

// setupChatHistory initializes the chat history in KV storage.
func setupChatHistory() error {
	// Check if history exists
	opt := guest.AlloyKvGet("chat:history")
	if opt.Set {
		return nil // History already exists
	}

	// Initialize empty history
	emptyHistory := []map[string]string{}
	historyData, err := json.Marshal(emptyHistory)
	if err != nil {
		return err
	}

	if !guest.AlloyKvSet("chat:history", historyData) {
		return guest.AlloyError("failed to set chat history")
	}

	return nil
}

// handleSendMessage handles a chat:send message.
func handleSendMessage(msg guest.AlloyMessage) guest.AlloyMessage {
	pluginID := "wit-chat-plugin"

	// Parse the message
	var payload struct {
		Text string `json:"text"`
		From string `json:"from"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return createErrorResponse(msg, "invalid_payload")
	}

	// Validate the message
	if payload.Text == "" || payload.From == "" {
		return createErrorResponse(msg, "missing_text_or_from")
	}

	// Get current history
	historyOpt := guest.AlloyKvGet("chat:history")
	if !historyOpt.Set {
		return createErrorResponse(msg, "failed_to_get_history")
	}

	var history []map[string]string
	if err := json.Unmarshal(historyOpt.Value, &history); err != nil {
		return createErrorResponse(msg, "failed_to_parse_history")
	}

	// Add the new message
	newMessage := map[string]string{
		"text": payload.Text,
		"from": payload.From,
		"time": timeNow(),
	}
	history = append(history, newMessage)

	// Save updated history
	historyData, err := json.Marshal(history)
	if err != nil {
		return createErrorResponse(msg, "failed_to_marshal_history")
	}

	if !guest.AlloyKvSet("chat:history", historyData) {
		return createErrorResponse(msg, "failed_to_save_history")
	}

	// Broadcast the message to all clients
	broadcastMsg := guest.AlloyMessage{
		Method:  "chat:receive",
		Sender:  pluginID,
		Payload: msg.Payload,
	}
	guest.AlloyRouteMessage(broadcastMsg)

	// Create success response
	respPayload := map[string]string{"status": "sent", "time": timeNow()}
	respData, _ := json.Marshal(respPayload)

	return guest.AlloyMessage{
		Id:      msg.Id + "-response",
		Method:  msg.Method,
		Sender:  pluginID,
		Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
		Payload: respData,
	}
}

// handleHistoryRequest handles a chat:history message.
func handleHistoryRequest(msg guest.AlloyMessage) guest.AlloyMessage {
	pluginID := "wit-chat-plugin"

	// Get current history
	historyOpt := guest.AlloyKvGet("chat:history")
	if !historyOpt.Set {
		return createErrorResponse(msg, "failed_to_get_history")
	}

	// Create response with history
	resp := guest.AlloyMessage{
		Id:      msg.Id + "-response",
		Method:  msg.Method,
		Sender:  pluginID,
		Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
		Payload: historyOpt.Value,
	}

	return resp
}

// createErrorResponse creates an error response.
func createErrorResponse(msg guest.AlloyMessage, errorMsg string) guest.AlloyMessage {
	errData := map[string]string{"error": errorMsg}
	data, _ := json.Marshal(errData)

	return guest.AlloyMessage{
		Id:      msg.Id + "-response",
		Method:  msg.Method,
		Sender:  "wit-chat-plugin",
		Target:  guest.AlloyOption[string]{Value: msg.Sender, Set: true},
		Payload: data,
	}
}

// timeNow returns the current time as a string.
func timeNow() string {
	// In a real implementation, we would get the time from the host
	return "2026-03-21T12:00:00Z"
}

// AlloyError creates an error for logging.
func AlloyError(msg string) error {
	guest.AlloyLog("error", msg)
	return fmt.Errorf(msg)
}