package main

import (
	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

func init() {
	wasm.SetHandler(handleMessage)
}

func main() {}

func handleMessage(msg wasm.Message) wasm.Message {
	switch msg.Method {
	case "check":
		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-iam",
			Target:  msg.Sender,
			Payload: []byte(`{"allowed":true}`),
		}
	default:
		return wasm.Message{}
	}
}
