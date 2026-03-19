package main

import (
	"encoding/json"
	"fmt"
	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

func init() {
	wasm.SetHandler(handleMessage)
}

func main() {}

func handleMessage(msg wasm.Message) wasm.Message {
	switch msg.Method {
	case "open":
		var req struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		json.Unmarshal(msg.Payload, &req)
		
		// In a real plugin, this would track open files/buffers
		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-buffer-manager",
			Target:  msg.Sender,
			Payload: []byte(fmt.Sprintf(`{"status":"opened","id":"%s"}`, req.ID)),
		}

	case "list":
		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-buffer-manager",
			Target:  msg.Sender,
			Payload: []byte(`{"buffers":[]}`),
		}

	default:
		return wasm.Message{}
	}
}
