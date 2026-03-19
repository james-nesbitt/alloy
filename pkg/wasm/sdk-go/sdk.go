//go:build tinygo || wasip1 || wasm
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

//go:wasmimport alloy log
func alloyLog(ptr uint32, size uint32)

//go:wasmimport alloy kv_set
func alloyKVSet(kPtr, kLen, vPtr, vLen uint32) uint32

//go:wasmimport alloy kv_get
func alloyKVGet(kPtr, kLen, vPtr, vMaxLen uint32) uint32

//go:wasmimport alloy kv_delete
func alloyKVDelete(kPtr, kLen uint32) uint32

//go:wasmimport alloy route_message
func alloyRouteMessage(ptr uint32, size uint32) uint32

//go:wasmimport alloy sleep_forever
func alloySleepForever()

// Log sends a string to the host's logger.
func Log(msg string) {
	ptr := uintptr(unsafe.Pointer(unsafe.StringData(msg)))
	alloyLog(uint32(ptr), uint32(len(msg)))
}

// KVSet stores data in the host's durable KV store.
func KVSet(key string, value []byte) bool {
	kPtr := uintptr(unsafe.Pointer(unsafe.StringData(key)))
	vPtr := uintptr(unsafe.Pointer(&value[0]))
	return alloyKVSet(uint32(kPtr), uint32(len(key)), uint32(vPtr), uint32(len(value))) == 0
}

// KVGet retrieves data from the host's durable KV store.
func KVGet(key string) []byte {
	kPtr := uintptr(unsafe.Pointer(unsafe.StringData(key)))

	// 1. Determine size
	size := alloyKVGet(uint32(kPtr), uint32(len(key)), 0, 0)
	if size == 0 {
		return nil
	}

	// 2. Read into buffer
	buf := make([]byte, size)
	vPtr := uintptr(unsafe.Pointer(&buf[0]))
	actual := alloyKVGet(uint32(kPtr), uint32(len(key)), uint32(vPtr), size)
	if actual != size {
		return nil
	}

	return buf
}

// KVDelete removes a key from the host's durable KV store.
func KVDelete(key string) bool {
	kPtr := uintptr(unsafe.Pointer(unsafe.StringData(key)))
	return alloyKVDelete(uint32(kPtr), uint32(len(key))) == 0
}

// SleepForever blocks the plugin execution safely using a host-side block.
func SleepForever() {
	alloySleepForever()
}

// RouteMessage sends a message to the host kernel for routing.
func RouteMessage(msg Message) bool {
	data, err := json.Marshal(msg)
	if err != nil {
		return false
	}
	ptr := uintptr(unsafe.Pointer(&data[0]))
	return alloyRouteMessage(uint32(ptr), uint32(len(data))) == 0
}

var allocations = make(map[uintptr][]byte)

// alloy_malloc is exported for the host to allocate memory in the guest.
//export alloy_malloc
//go:wasmexport alloy_malloc
func Alloy_malloc(size uint32) uintptr {
	buf := make([]byte, size)
	ptr := uintptr(unsafe.Pointer(&buf[0]))
	allocations[ptr] = buf
	return ptr
}

// alloy_free is exported for the host to free memory in the guest.
//export alloy_free
//go:wasmexport alloy_free
func Alloy_free(ptr uintptr) {
	delete(allocations, ptr)
}

// alloy_handle_message is exported for the host to send messages to the guest.
//export alloy_handle_message
//go:wasmexport alloy_handle_message
func Alloy_handle_message(ptr uintptr, size uint32) uint64 {
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

	// 4. Copy response to an allocated buffer to ensure it's stable for the host
	respSize := uint32(len(respBuf))
	outPtr := Alloy_malloc(respSize)
	outBuf := unsafe.Slice((*byte)(unsafe.Pointer(outPtr)), respSize)
	copy(outBuf, respBuf)

	// 5. Return packed offset and size
	return uint64(uintptr(outPtr))<<32 | uint64(respSize)
}
