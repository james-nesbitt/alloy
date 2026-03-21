//go:build tinygo || wasip1 || wasm
package wasm

import (
	"encoding/json"
	"unsafe"
)

// Message is a copy of the api.Message to avoid circular dependencies.
type Message struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Sender    string          `json:"sender"`
	Target    string          `json:"target,omitempty"`
	Method    string          `json:"method"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp int64           `json:"timestamp"`
	Actor     string          `json:"actor,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
}

// Capability describes a functionality provided by a component.
type Capability struct {
	Method      string            `json:"method,omitempty"`
	Description string            `json:"description,omitempty"`
	Shortcut    string            `json:"shortcut,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type MessageHandler func(msg Message) Message

var (
	handler      MessageHandler
	capabilities []Capability
)

// SetHandler registers the plugin's message handler.
func SetHandler(h MessageHandler) {
	handler = h
}

// SetCapabilities registers the plugin's capabilities.
func SetCapabilities(caps []Capability) {
	capabilities = caps
}

// ... original alloy_malloc and other exports ...

// alloy_capabilities is exported for the host to query the plugin's capabilities.
//export alloy_capabilities
//go:wasmexport alloy_capabilities
func Alloy_capabilities() uint64 {
	if capabilities == nil {
		return 0
	}
	data, err := json.Marshal(capabilities)
	if err != nil {
		return 0
	}
	ptr := Alloy_malloc(uint32(len(data)))
	if ptr == 0 {
		return 0
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), uint32(len(data)))
	copy(buf, data)
	return uint64(uintptr(ptr))<<32 | uint64(len(data))
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

//go:wasmimport alloy fetch
func alloyFetch(reqPtr, reqSize, respPtrPtr, respSizePtr uint32) uint32

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
	if len(value) == 0 {
		return alloyKVSet(uint32(kPtr), uint32(len(key)), 0, 0) == 0
	}
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

// Fetch performs an HTTP request via the host.
func Fetch(req FetchRequest) (*FetchResponse, error) {
	data, _ := json.Marshal(req)
	var respPtr, respSize uint32

	res := alloyFetch(
		uint32(uintptr(unsafe.Pointer(&data[0]))),
		uint32(len(data)),
		uint32(uintptr(unsafe.Pointer(&respPtr))),
		uint32(uintptr(unsafe.Pointer(&respSize))),
	)

	if res != 0 {
		return nil, json.Unmarshal([]byte(`{"error":"fetch_failed"}`), &struct{}{}) // Just a placeholder error
	}

	// Read response data from allocated memory
	respBuf := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(respPtr))), respSize)
	var resp FetchResponse
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return nil, err
	}

	// Free the memory allocated by the host
	Alloy_free(uintptr(respPtr))

	return &resp, nil
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

// Memory management without sync.Mutex (rely on host-side serialization)
var allocations = make(map[uintptr][]byte)

// alloy_malloc is exported for the host to allocate memory in the guest.
//export alloy_malloc
//go:wasmexport alloy_malloc
func Alloy_malloc(size uint32) uintptr {
	if size == 0 {
		return 0
	}
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
	if resp.Type == "ignore" {
		return 0
	}

	// 3. Serialize response
	respBuf, err := json.Marshal(resp)
	if err != nil {
		return 0
	}

	// 4. Copy response to an allocated buffer to ensure it's stable for the host
	respSize := uint32(len(respBuf))
	outPtr := Alloy_malloc(respSize)
	if outPtr == 0 {
		return 0
	}
	outBuf := unsafe.Slice((*byte)(unsafe.Pointer(outPtr)), respSize)
	copy(outBuf, respBuf)

	// 5. Return packed offset and size
	return uint64(uintptr(outPtr))<<32 | uint64(respSize)
}
