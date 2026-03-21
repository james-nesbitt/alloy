//go:build wasip1

package main

import (
    "time"
    "github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

func main() {
    p := wasm.New("plugin-mock")
    p.DefaultHandle(handleMessage)
    p.Run()
}

func handleMessage(msg wasm.Message) wasm.Message {
    wasm.Log("Mock handling message: " + msg.ID)
    return wasm.Message{
        ID: msg.ID + "-resp",
        Type: "response",
        Sender: "mock-plugin",
        Target: msg.Sender,
        Payload: []byte(`{"status":"mock-ready"}`),
        Timestamp: time.Now().Unix(),
    }
}
