package main

import (
	"encoding/json"
	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

func main() {
	wasm.SetHandler(handleMessage)
}

// malloc is needed for the host to allocate memory in the guest

func handleMessage(msg wasm.Message) wasm.Message {
	switch msg.Method {
	case "get_secret":
		var req struct {
			ID string `json:"id"`
		}
		json.Unmarshal(msg.Payload, &req)

		val := wasm.KVGet("secret:" + req.ID)
		if val == nil {
			return wasm.Message{
				ID:     msg.ID + "-resp",
				Type:   "response",
				Sender: "plugin-secrets",
				Target: msg.Sender,
				Payload: []byte(`{"error":"secret not found"}`),
			}
		}

		return wasm.Message{
			ID:     msg.ID + "-resp",
			Type:   "response",
			Sender: "plugin-secrets",
			Target: msg.Sender,
			Payload: []byte(`{"value":"` + string(val) + `"}`),
		}

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

	default:
		return wasm.Message{}
	}
}
