package main

import (
	"encoding/json"
	"time"

	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

func main() {
	wasm.SetHandler(handleMessage)
}

// malloc is needed for the host to allocate memory in the guest

func handleMessage(msg wasm.Message) wasm.Message {
	switch msg.Method {
	case "status":
		status := map[string]any{
			"status": "healthy",
			"uptime": "wasm-monitored",
			"source": "wasm",
		}
		payload, _ := json.Marshal(status)
		return wasm.Message{
			ID:        msg.ID + "-resp",
			Type:      "response",
			Sender:    "plugin-health",
			Target:    msg.Sender,
			Payload:   payload,
			Timestamp: time.Now().Unix(),
		}
	default:
		return wasm.Message{}
	}
}
