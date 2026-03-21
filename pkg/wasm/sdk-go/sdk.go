//go:build tinygo || wasip1 || wasm
package wasm

import (
	"encoding/json"
	"fmt"
	"unsafe"
)

// Global state variables for the plugin.
var (
	handler      MessageHandler
	capabilities []Capability
    
    // pinned holds references to allocated buffers to prevent GC collection
    pinned = make(map[uint32][]byte)

    // Global buffers for message passing to avoid allocation in exports
    inBuffer  [1024 * 1024]byte
    outBuffer [1024 * 1024]byte
)

// SetHandler registers the plugin's message handler.
func SetHandler(h MessageHandler) {
	handler = h
}

// SetCapabilities registers the plugin's capabilities.
func SetCapabilities(caps []Capability) {
	capabilities = caps
}

//go:wasmimport alloy log
func alloyLog(ptr uint32, size uint32)

//go:wasmimport alloy kv_set
func alloyKVSet(kPtr, kLen, vPtr, vLen uint32) uint32

//go:wasmimport alloy kv_get
func alloyKVGet(kPtr, kLen, vPtr, vMaxLen uint32) uint32

//go:wasmimport alloy kv_delete
func alloyKVDelete(kPtr, kLen uint32) uint32

//go:wasmimport alloy kv_list
func alloyKVList(pPtr, pLen, respPtrPtr, respSizePtr uint32) uint32

//go:wasmimport alloy route_message
func alloyRouteMessage(ptr uint32, size uint32)

//go:wasmimport alloy fetch
func alloyFetch(reqPtr, reqSize, respPtrPtr, respSizePtr uint32) uint32

//go:wasmimport alloy started
func alloyStarted(inPtr, outPtr uint32)

//go:wasmimport alloy sleep_forever
func alloySleepForever()

//go:wasmimport alloy yield
func alloyYield()

//go:wasmimport alloy call
func alloyCall(ptr, size, respPtrPtr, respSizePtr uint32) uint32

// PluginStarted signals to the host that the plugin has finished initialization.
func PluginStarted() {
	inPtr := uint32(uintptr(unsafe.Pointer(&inBuffer[0])))
	outPtr := uint32(uintptr(unsafe.Pointer(&outBuffer[0])))
	alloyStarted(inPtr, outPtr)
}

// SleepForever blocks the main goroutine indefinitely via a host call.
func SleepForever() {
	alloySleepForever()
}

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

// KVList retrieves all keys matching a prefix from the host's durable KV store.
func KVList(prefix string) []string {
	pPtr := uintptr(unsafe.Pointer(unsafe.StringData(prefix)))
	var respPtr, respSize uint32

	res := alloyKVList(
		uint32(pPtr),
		uint32(len(prefix)),
		uint32(uintptr(unsafe.Pointer(&respPtr))),
		uint32(uintptr(unsafe.Pointer(&respSize))),
	)

	if res != 0 {
		return nil
	}

	respBuf := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(respPtr))), respSize)
	var keys []string
	if err := json.Unmarshal(respBuf, &keys); err != nil {
		return nil
	}

	return keys
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
		// Mock error response for failed fetch
		return nil, json.Unmarshal([]byte(`{"error":"fetch_failed"}`), &struct{}{})
	}

	// Read response data from allocated memory
	respBuf := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(respPtr))), respSize)
	var resp FetchResponse
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// RouteMessage sends a message to the host kernel for routing.
func RouteMessage(msg Message) bool {
	data, err := json.Marshal(msg)
	if err != nil {
		return false
	}
	ptr := uintptr(unsafe.Pointer(&data[0]))
	alloyRouteMessage(uint32(ptr), uint32(len(data)))
	return true
}

// Call performs a synchronous call to another plugin.
func Call(msg Message) (Message, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return Message{}, err
	}

	var respPtr, respSize uint32
	res := alloyCall(
		uint32(uintptr(unsafe.Pointer(&data[0]))),
		uint32(len(data)),
		uint32(uintptr(unsafe.Pointer(&respPtr))),
		uint32(uintptr(unsafe.Pointer(&respSize))),
	)

	if res != 0 {
		return Message{}, fmt.Errorf("call failed")
	}

	respBuf := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(respPtr))), respSize)
	var resp Message
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return Message{}, err
	}

	return resp, nil
}

// alloy_get_in_ptr returns the address of the input buffer.
//go:wasmexport alloy_get_in_ptr
func Alloy_get_in_ptr() uint32 {
	return uint32(uintptr(unsafe.Pointer(&inBuffer[0])))
}

// alloy_get_out_ptr returns the address of the output buffer.
//go:wasmexport alloy_get_out_ptr
func Alloy_get_out_ptr() uint32 {
	return uint32(uintptr(unsafe.Pointer(&outBuffer[0])))
}

// alloy_malloc is exported for the host to allocate memory in the guest.
//go:wasmexport alloy_malloc
func Alloy_malloc(size uint32) uint32 {
	if size == 0 {
		return 0
	}
	buf := make([]byte, size)
	ptr := uint32(uintptr(unsafe.Pointer(&buf[0])))
	pinned[ptr] = buf
	return ptr
}

// alloy_free is exported for the host to free memory in the guest.
//go:wasmexport alloy_free
func Alloy_free(ptr uint32) {
	delete(pinned, ptr)
}

// alloy_handle_message is exported for the host to send messages to the guest.
//go:wasmexport alloy_handle_message
func Alloy_handle_message(size uint32) uint32 {
	if size == 0 {
		return 0
	}
	if size > uint32(len(inBuffer)) {
		return 0
	}
	// 1. Read message from guest memory (inBuffer)
	buf := inBuffer[:size]
	var msg Message
	if err := json.Unmarshal(buf, &msg); err != nil {
		Log("Failed to unmarshal message in guest: " + err.Error())
		return 0
	}

	// 2. Call the registered handler
	if handler == nil {
		Log("Guest handler is nil")
		return 0
	}
	Log("Calling guest handler...")
	resp := handler(msg)
	if resp.ID == "" && resp.Target == "" {
		Log("Guest handler returned empty response")
		return 0
	}
	Log("Guest handler returned response: " + resp.Method)

	// 3. Serialize response
	respBuf, err := json.Marshal(resp)
	if err != nil {
		Log("Failed to marshal response in guest: " + err.Error())
		return 0
	}

	if len(respBuf) > len(outBuffer) {
		Log("Response too large for guest outBuffer")
		return 0
	}

	// 4. Copy response to output buffer
	copy(outBuffer[:], respBuf)

	// 5. Return size
	return uint32(len(respBuf))
}

// alloy_capabilities is exported for the host to query the plugin's capabilities.
//go:wasmexport alloy_capabilities
func Alloy_capabilities() uint32 {
	if capabilities == nil {
		return 0
	}
	data, err := json.Marshal(capabilities)
	if err != nil {
		return 0
	}
	if len(data) > len(outBuffer) {
		return 0
	}
	copy(outBuffer[:], data)
	return uint32(len(data))
}
