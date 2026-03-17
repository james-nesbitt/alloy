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
	case "schedule":
		return wasm.Message{
			ID:        msg.ID + "-resp",
			Type:      "response",
			Sender:    "plugin-tasks",
			Target:    msg.Sender,
			Payload:   []byte(`{"task_id":"wasm-task-1"}`),
			Timestamp: time.Now().Unix(),
		}
	default:
		return wasm.Message{}
	}
}
