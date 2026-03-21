//go:build !tinygo && !wasip1 && !wasm
package wasm

import (
	"encoding/json"
	"fmt"
)

// Message is a copy of api.Message.
type Message struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Sender    string          `json:"sender"`
	Target    string          `json:"target,omitempty"`
	Method    string          `json:"method"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

// Capability describes a functionality provided by a component.
type Capability struct {
	Method      string            `json:"method,omitempty"`
	Description string            `json:"description,omitempty"`
	Shortcut    string            `json:"shortcut,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type MessageHandler func(msg Message) Message
type FetchHandler func(req FetchRequest) (*FetchResponse, error)

var (
	handler      MessageHandler
	capabilities []Capability
	kv           = make(map[string][]byte)
	routes       []Message
	fetcher      FetchHandler
)

// SetHandler registers the plugin's message handler.
func SetHandler(h MessageHandler) { handler = h }

func SetCapabilities(caps []Capability) { capabilities = caps }

func Log(msg string) { fmt.Printf("[PLUGIN MOCK LOG] %s\n", msg) }

func KVSet(key string, value []byte) bool {
	kv[key] = value
	return true
}

func KVGet(key string) []byte { return kv[key] }

func KVDelete(key string) bool {
	delete(kv, key)
	return true
}

func SleepForever() { /* No-op in mock mode */ }

type FetchRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
}

type FetchResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body"`
}

func Fetch(req FetchRequest) (*FetchResponse, error) {
	if fetcher != nil {
		return fetcher(req)
	}
	return nil, fmt.Errorf("mock: no fetcher configured")
}

func RouteMessage(msg Message) bool {
	routes = append(routes, msg)
	return true
}

// Memory management exports - no-ops for mock
func Alloy_malloc(size uint32) uintptr { return 0 }
func Alloy_free(ptr uintptr)          {}

// Mock Testing Helpers (Not available in real WASM)

func ResetMock() {
	kv = make(map[string][]byte)
	routes = nil
	fetcher = nil
}

func SetFetchHandler(h FetchHandler) { fetcher = h }
func GetKV() map[string][]byte       { return kv }
func GetRoutedMessages() []Message   { return routes }

func SimulateMessage(m Message) Message {
	if handler == nil {
		return Message{}
	}
	return handler(m)
}

func (p *Plugin) MockSimulate(msg Message) Message {
	return p.dispatch(msg)
}
