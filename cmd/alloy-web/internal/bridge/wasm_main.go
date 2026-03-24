package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"syscall/js"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

type WASMFrontend struct {
	commandTree *frontend.CommandNode
}

func main() {
	fmt.Println("Alloy: Browser-WASM frontend initializing...")
	
	wf := &WASMFrontend{}

	// Register JS callbacks
	alloy := js.Global().Get("alloy")
	alloy.Set("send", js.FuncOf(wf.jsSend))
	alloy.Set("search", js.FuncOf(wf.jsSearch))
	alloy.Set("handleEvent", js.FuncOf(wf.jsHandleEvent))

	// Fetch initial command tree
	go wf.fetchCommands()

	// Keep alive
	select {}
}

func (wf *WASMFrontend) fetchCommands() {
	resp, err := http.Get("/api/commands")
	if err != nil {
		fmt.Printf("WASM Trace: Failed to fetch commands: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var regs []api.Registration
	if err := json.Unmarshal(body, &regs); err != nil {
		fmt.Printf("WASM Trace: Failed to unmarshal registrations: %v\n", err)
		return
	}

	wf.commandTree = frontend.BuildCommandTree(regs)
	fmt.Println("WASM Trace: Command tree built successfully.")
}

func (wf *WASMFrontend) jsHandleEvent(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return nil
	}
	
	var msg api.Message
	if err := json.Unmarshal([]byte(args[0].String()), &msg); err != nil {
		return nil
	}

	// Update local state based on event
	// For example, if a new plugin is registered, we could re-fetch commands
	if msg.Target == "system" && msg.Method == "discovery:registered" {
		go wf.fetchCommands()
	}

	return nil
}

func (wf *WASMFrontend) jsSend(this js.Value, args []js.Value) any {
	// ... (no changes needed to previous logic for POST /api/send)
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
	
	if wf.commandTree == nil {
		return "[]"
	}

	flattened := wf.commandTree.Flatten("")
	var results []frontend.SearchItem
	for _, item := range flattened {
		score := frontend.FuzzyScore(item.FullTitle, query)
		if score > 0 {
			item.Weight = score
			results = append(results, item)
		}
	}
	
	frontend.SortItems(results)
	if len(results) > 10 {
		results = results[:10]
	}
	
	resJson, _ := json.Marshal(results)
	return string(resJson)
}
