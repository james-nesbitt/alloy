package main

import (
	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

func init() {
	p := wasm.New("plugin-health").
		WithCapability("status", "Get the health status of this WASM instance", "h s").
		Handle("status", func(msg wasm.Message) wasm.Message {
			status := map[string]any{
				"status": "healthy",
				"uptime": "wasm-monitored",
				"source": "wasm-v2",
			}
			return wasm.Reply(msg, status)
		})

	p.Run()
}

func main() {}
