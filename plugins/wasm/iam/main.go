package main

import (
	"time"

	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

func main() {
	wasm.SetHandler(handleMessage)
}

// malloc is needed for the host to allocate memory in the guest
//go:export malloc
func malloc(size uint32) uintptr {
	return wasm.Malloc(size)
}

func handleMessage(msg wasm.Message) wasm.Message {
	switch msg.Method {
	case "authorize":
		// WASM-based authorization logic
		// In a real scenario, this might check a policy file or KV store
		return wasm.Message{
			ID:        msg.ID + "-resp",
			Type:      "response",
			Sender:    "plugin-iam",
			Target:    msg.Sender,
			Payload:   []byte(`{"allowed":true,"provider":"wasm"}`),
			Timestamp: time.Now().Unix(),
		}
	default:
		return wasm.Message{}
	}
}
