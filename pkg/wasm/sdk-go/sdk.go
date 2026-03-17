package wasm

import (
	"encoding/json"
	"unsafe"
)

// Message is a copy of the api.Message to avoid circular dependencies if needed,
// but usually the SDK is a standalone package.
type Message struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Sender    string          `json:"sender"`
	Target    string          `json:"target,omitempty"`
	Method    string          `json:"method"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

type MessageHandler func(msg Message) Message

var handler MessageHandler

// SetHandler registers the plugin's message handler.
func SetHandler(h MessageHandler) {
	handler = h
}

// export malloc for the host to allocate memory in guest
//
//go:export malloc
func malloc(size uint32) uintptr {
	ptr := make([]byte, size)
	return uintptr(unsafe.Pointer(&ptr[0]))
}

//go:export alloy_handle_message
func alloyHandleMessage(ptr uintptr, size uint32) uint64 {
	// 1. Read message from guest memory
	buf := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size)
	var msg Message
	if err := json.Unmarshal(buf, &msg); err != nil {
		return 0
	}

	// 2. Call the registered handler
	if handler == nil {
		return 0
	}
	resp := handler(msg)

	// 3. Serialize response
	respBuf, err := json.Marshal(resp)
	if err != nil {
		return 0
	}

	// 4. Return packed offset and size
	respPtr := uintptr(unsafe.Pointer(&respBuf[0]))
	return uint64(uintptr(respPtr))<<32 | uint64(len(respBuf))
}
