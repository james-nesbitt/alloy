package main

import (
	"encoding/json"
	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

func main() {
	p := wasm.New("plugin-secrets").
		WithCapability("store_secret", "Store a new secret", "s s").
		WithCapability("get_secret", "Retrieve a secret by ID", "s g")

	p.Handle("store_secret", func(msg wasm.Message) wasm.Message {
		var req struct {
			ID    string `json:"id"`
			Value string `json:"value"`
		}
		json.Unmarshal(msg.Payload, &req)
		wasm.KVSet("secret:"+req.ID, []byte(req.Value))
		return wasm.Reply(msg, map[string]string{"status": "stored"})
	})

	p.Handle("get_secret", func(msg wasm.Message) wasm.Message {
		var req struct {
			ID string `json:"id"`
		}
		json.Unmarshal(msg.Payload, &req)
		val := wasm.KVGet("secret:" + req.ID)
		return wasm.Reply(msg, map[string]string{"value": string(val)})
	})

	p.Run()
}
