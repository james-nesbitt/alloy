package main

import (
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
	case "create":
		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-tasks",
			Target:  msg.Sender,
			Payload: []byte(`{"status":"created"}`),
		}
	case "list":
		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-tasks",
			Target:  msg.Sender,
			Payload: []byte(`{"tasks":[]}`),
		}
	default:
		return wasm.Message{}
	}
}
