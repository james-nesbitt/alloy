package main

import (
	"encoding/json"
	"fmt"
	"log"
	"syscall/js"

	"github.com/james-nesbitt/alloy/pkg/frontend"
)

type WASMFrontend struct {
	client      *frontend.Client
	commandTree *frontend.CommandNode
}

func main() {
	fmt.Println("Alloy: Browser-WASM frontend initializing...")
	
	wf := &WASMFrontend{}

	// Register JS callbacks
	js.Global().Get("alloy").Set("send", js.FuncOf(wf.jsSend))
	js.Global().Get("alloy").Set("search", js.FuncOf(wf.jsSearch))

	// Keep alive
	select {}
}

func (wf *WASMFrontend) jsSend(this js.Value, args []js.Value) any {
	if len(args) < 2 {
		return "Missing arguments: target, method [, payload]"
	}

	target := args[0].String()
	method := args[1].String()
	payload := ""
	if len(args) > 2 {
		payload = args[2].String()
	}

	fmt.Printf("WASM Bridge: Sending %s %s with %s\n", target, method, payload)
	
	// API call to local proxy
	go func() {
		// Mock implementation: eventually posts back to /api/send
		log.Printf("Proxy Send: %s %s -> %s\n", target, method, payload)
		
		// Map back to Alloy Kernel protocol
		_ = js.Global().Get("fetch").Call("call", js.Global(), "/api/send", js.ValueOf(map[string]any{
			"method": "POST",
			"body":   fmt.Sprintf(`{"target": "%s", "method": "%s", "payload": "%s"}`, target, method, payload),
		}))
	}()

	return nil
}

func (wf *WASMFrontend) jsSearch(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return nil
	}
	query := args[0].String()
	
	// Mock search results
	results := []map[string]any{
		{"Display": "Project Open", "Raw": "project open", "Score": 100},
		{"Display": "AI Chat", "Raw": "ai chat", "Score": 80},
	}
	
	fmt.Printf("WASM Bridge: Searching for %s\n", query)
	
	resJson, _ := json.Marshal(results)
	return string(resJson)
}
