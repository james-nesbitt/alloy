package main

import (
    "time"
    "github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

func main() {
    wasm.SetHandler(handleMessage)
}

func handleMessage(msg wasm.Message) wasm.Message {
    return wasm.Message{
        ID: msg.ID + "-resp",
        Type: "response",
        Sender: "mock-plugin",
        Target: msg.Sender,
        Payload: []byte(`{"status":"mock-ready"}`),
        Timestamp: time.Now().Unix(),
    }
}
