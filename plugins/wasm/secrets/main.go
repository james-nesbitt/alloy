package main

import (
	"encoding/json"
	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

func init() {
	wasm.SetHandler(handleMessage)
	wasm.SetCapabilities([]wasm.Capability{
		{Method: "store_secret", Description: "Store a new secret", Shortcut: "s s", Annotations: map[string]string{"group": "secrets"}},
		{Method: "get_secret", Description: "Retrieve a secret by ID", Shortcut: "s g", Annotations: map[string]string{"group": "secrets"}},
	})
}

func main() {
	wasm.SleepForever()
}

func handleMessage(msg wasm.Message) wasm.Message {
	switch msg.Method {
	case "store_secret":
		var req struct {
			ID    string `json:"id"`
			Value string `json:"value"`
		}
		json.Unmarshal(msg.Payload, &req)
		wasm.KVSet("secret:"+req.ID, []byte(req.Value))
		return wasm.Message{
			ID:     msg.ID + "-resp",
			Type:   "response",
			Sender: "plugin-secrets",
			Target: msg.Sender,
			Payload: []byte(`{"status":"stored"}`),
		}
	case "get_secret":
		var req struct {
			ID string `json:"id"`
		}
		json.Unmarshal(msg.Payload, &req)
		val := wasm.KVGet("secret:" + req.ID)
		return wasm.Message{
			ID:     msg.ID + "-resp",
			Type:   "response",
			Sender: "plugin-secrets",
			Target: msg.Sender,
			Payload: []byte(`{"value":"` + string(val) + `"}`),
		}
	default:
		return wasm.Message{}
	}
}
