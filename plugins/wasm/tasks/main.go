package main

import (
	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

func main() {
	p := wasm.New("plugin-tasks").
		WithCapability("create", "Create a new task", "t c").
		WithCapability("list", "List all tasks", "t l")

	p.Handle("create", func(msg wasm.Message) wasm.Message {
		return wasm.Reply(msg, map[string]string{"status": "created"})
	})

	p.Handle("list", func(msg wasm.Message) wasm.Message {
		return wasm.Reply(msg, map[string]any{"tasks": []any{}})
	})

	p.Run()
}
