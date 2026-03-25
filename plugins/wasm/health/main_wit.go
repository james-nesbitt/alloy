//go:build wasip1 || wasm

package main

import (
	"encoding/json"
	"github.com/james-nesbitt/alloy/pkg/wasm/guest"
)

func main() {
	plugin := guest.NewPlugin("health")

	plugin.RegisterMethod("status", "Get the health status of this WASM instance", func(msg guest.Message) *guest.Message {
		status := map[string]string{
			"status": "healthy",
			"uptime": "wasm-monitored",
			"source": "wasm-sdk-v2",
		}

		payload, _ := json.Marshal(status)
		return &guest.AlloyMessage{
			Id:      msg.Id + "-resp",
			Method:  msg.Method,
			Payload: payload,
			Target:  msg.Target,
		}
	})

	plugin.Log(guest.LogLevelInfo, "Health plugin (SDK v2.0) initialized")
	plugin.Serve()
}
