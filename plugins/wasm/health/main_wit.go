//go:build wasip1 || wasm

package main

import (
	. "github.com/jnesbitt/alloy-go/pkg/wasm/bindings/guest"
	"encoding/json"
)

func main() {
	// Initialize the plugin
	AlloyInit(
		"health",
		[]AlloyCapability{
			{Method: "status", Description: "Get the health status of this WASM instance"},
		},
	)

	AlloyLog("info", "Health plugin initialized")

	// Main message loop
	for {
		msgOption := AlloyGetNextMessage()
		if msgOption.IsNone() {
			continue
		}

		msg := msgOption.Unwrap()
		if msg.Method == "status" {
			// Create response payload
			status := map[string]string{
				"status": "healthy",
				"uptime": "wasm-monitored",
				"source": "wasm-wit",
			}

			payload, err := json.Marshal(status)
			if err != nil {
				AlloyLog("error", "Failed to marshal status: "+err.Error())
				continue
			}

			// Create and send response
			resp := AlloyMessage{
				Id:        msg.Id + "-response",
				Method:    msg.Method,
				Sender:    "health",
				Target:    Some(msg.Sender),
				Payload:   payload,
				Timestamp: 0,
			}
			AlloySendResponse(resp)
		}
	}
}
