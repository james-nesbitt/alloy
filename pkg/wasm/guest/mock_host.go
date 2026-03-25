package guest

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MockHost implements HostInterface by storing state in memory.
type MockHost struct {
	ID           string
	Capabilities []AlloyCapability
	StartedSig   bool
	LastLogs     []string
	
	// KV Store
	KV map[string][]byte
	
	// Registry
	Providers map[string][]string // method -> plugin IDs
	AllCaps   []AlloyCapability
	
	// Buffers
	Buffers map[string]AlloyBuffer
	
	// Workspaces
	Workspaces      map[string]AlloyWorkspace
	ActiveWorkspace string
	
	// Message Queue for "simulate" mode
	IncomingMessages chan AlloyMessage
	Responses        map[string]AlloyMessage
	
	mu sync.RWMutex
}

func NewMockHost() *MockHost {
	return &MockHost{
		KV:               make(map[string][]byte),
		Providers:        make(map[string][]string),
		Buffers:          make(map[string]AlloyBuffer),
		Workspaces:       make(map[string]AlloyWorkspace),
		IncomingMessages: make(chan AlloyMessage, 100),
		Responses:        make(map[string]AlloyMessage),
	}
}

func (m *MockHost) Init(id string, caps []AlloyCapability) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ID = id
	m.Capabilities = caps
}

func (m *MockHost) Started() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StartedSig = true
}

func (m *MockHost) GetNextMessage() Option[AlloyMessage] {
	msg, ok := <-m.IncomingMessages
	if !ok {
		return None[AlloyMessage]()
	}
	return Some(msg)
}

func (m *MockHost) SendResponse(msg AlloyMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses[msg.Id] = msg
}

func (m *MockHost) Log(level string, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastLogs = append(m.LastLogs, fmt.Sprintf("[%s] %s", level, msg))
}

func (m *MockHost) RouteMessage(msg AlloyMessage) {
	// In mock mode, we just log routing attempts for now
	m.Log("debug", fmt.Sprintf("Routed message %s to %s", msg.Id, msg.Target.Unwrap()))
}

func (m *MockHost) Call(msg AlloyMessage) AlloyMessage {
	// For simulation, we might want to provide pre-canned responses
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Simple mock behavior: if target is "kv", handle it locally
	target := ""
	if msg.Target.IsSome() {
		target = msg.Target.Unwrap()
	}

	if target == "kv" {
		return m.handleKVCall(msg)
	}
	
	return AlloyMessage{Id: msg.Id + "-resp", MsgType: "error", Method: "not-implemented", Payload: []byte("Mock call not implemented")}
}

func (m *MockHost) handleKVCall(msg AlloyMessage) AlloyMessage {
	switch msg.Method {
	case "get":
		var key string
		_ = json.Unmarshal(msg.Payload, &key)
		val, ok := m.KV[key]
		if !ok {
			return AlloyMessage{Id: msg.Id + "-resp", Method: "get", Payload: nil}
		}
		return AlloyMessage{Id: msg.Id + "-resp", Method: "get", Payload: val}
	}
	return AlloyMessage{Id: msg.Id + "-resp", Method: "error"}
}

func (m *MockHost) KvSet(key string, val []byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.KV[key] = val
	return true
}

func (m *MockHost) KvGet(key string) Option[[]byte] {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.KV[key]
	if !ok {
		return None[[]byte]()
	}
	return Some(val)
}

func (m *MockHost) KvDelete(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.KV, key)
	return true
}

func (m *MockHost) KvList(prefix string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var keys []string
	for k := range m.KV {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys
}

func (m *MockHost) GetActiveWorkspace() Option[AlloyWorkspace] {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ws, ok := m.Workspaces[m.ActiveWorkspace]
	if !ok {
		return None[AlloyWorkspace]()
	}
	return Some(ws)
}

func (m *MockHost) SetActiveWorkspace(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ActiveWorkspace = id
}

func (m *MockHost) ListWorkspaces() []AlloyWorkspace {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []AlloyWorkspace
	for _, ws := range m.Workspaces {
		list = append(list, ws)
	}
	return list
}

func (m *MockHost) RegisterWorkspace(ws AlloyWorkspace) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Workspaces[ws.Id] = ws
}

func (m *MockHost) UnregisterWorkspace(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Workspaces, id)
}

func (m *MockHost) RegisterCapability(cap AlloyCapability) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AllCaps = append(m.AllCaps, cap)
}

func (m *MockHost) UnregisterCapability(method string) {
	// Not implemented in mock
}

func (m *MockHost) FindProviders(method string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Providers[method]
}

func (m *MockHost) GetAllCapabilities() []AlloyCapability {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.AllCaps
}

func (m *MockHost) ReadBuffer(id string) Option[AlloyBuffer] {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.Buffers[id]
	if !ok {
		return None[AlloyBuffer]()
	}
	return Some(b)
}

func (m *MockHost) WriteBuffer(id string, content []byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Buffers[id] = AlloyBuffer{Id: id, Content: content, LastModified: uint64(time.Now().UnixNano())}
	return true
}

func (m *MockHost) ListBuffers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var ids []string
	for id := range m.Buffers {
		ids = append(ids, id)
	}
	return ids
}

func (m *MockHost) GetBufferView(id string) (ptr, size uint32, ok bool) {
	return 0, 0, false // Not sensible in mock mode
}

func (m *MockHost) RegisterWidget(w AlloyWidget) {
	m.Log("info", fmt.Sprintf("Registered widget: %s (%s)", w.Title, w.Id))
}

func (m *MockHost) UnregisterWidget(id string) {
	// Not implemented
}

func (m *MockHost) UpdateWidget(id string, content []byte) {
	// Not implemented
}

// NewMockPlugin creates a plugin instance using the MockHost.
// Useful for unit testing plugin logic on the host.
func NewMockPlugin(id string) (*Plugin, *MockHost) {
	host := NewMockHost()
	p := NewPlugin(id)
	p.host = host
	return p, host
}

// PushMessage injects a message into the mock host's incoming queue.
func (h *MockHost) PushMessage(msg AlloyMessage) {
	h.IncomingMessages <- msg
}

// GetResponse returns a response message for the given ID, if any.
func (h *MockHost) GetResponse(id string) (AlloyMessage, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	msg, ok := h.Responses[id]
	return msg, ok
}
