package main

import (
	"encoding/json"
	"time"

	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

func init() {
	wasm.SetHandler(handleMessage)
}

func main() {
	wasm.SleepForever()
}

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
