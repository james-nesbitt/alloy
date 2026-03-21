package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

type Buffer struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"` // ephemeral, file, stream
	MimeType     string                 `json:"mime_type"`
	Source       string                 `json:"source,omitempty"`
	ReadOnly     bool                   `json:"read_only"`
	BaseBufferID string                 `json:"base_buffer_id,omitempty"`
	Metadata     map[string]interface{} `json:"metadata"`
	Data         []byte                 `json:"-"`
	Timestamp    int64                  `json:"timestamp"`
}

var (
	buffers     = make(map[string]*Buffer)
	subscribers = make(map[string][]string)
	nextID      = 1
)

func findRootBuffer(id string) (*Buffer, bool) {
	curr := buffers[id]
	if curr == nil {
		return nil, false
	}
	// Prevent cycles and depth issues
	for i := 0; i < 20; i++ {
		if curr.BaseBufferID == "" {
			return curr, true
		}
		next, ok := buffers[curr.BaseBufferID]
		if !ok || next == nil {
			return curr, true
		}
		curr = next
	}
	return nil, false
}

func main() {
	p := wasm.New("plugin-buffer-manager").
		WithCapability("create", "Create a new buffer", "b c").
		WithCapability("list", "List all buffers", "b l").
		WithCapability("read", "Read buffer content", "b r").
		WithCapability("write", "Write buffer content", "b w").
		WithCapability("delete", "Delete a buffer", "b d").
		WithCapability("save", "Save buffer to persistent storage", "b s").
		WithCapability("load", "Load buffers from persistent storage", "b o")

	p.Handle("create", handleCreate)
	p.Handle("read", handleRead)
	p.Handle("write", handleWrite)
	p.Handle("append", handleAppend)
	p.Handle("list", handleList)
	p.Handle("delete", handleDelete)
	p.Handle("subscribe", handleSubscribe)
	p.Handle("clear", handleClear)
	p.Handle("save", handleSave)
	p.Handle("load", handleLoad)
	p.Handle("set_metadata", handleSetMetadata)

	p.Run()
}

func handleMessage(msg wasm.Message) wasm.Message {
	return wasm.Message{} // Legacy, not used with p.Run() but kept for now if others call it
}

func handleSubscribe(msg wasm.Message) wasm.Message {
	var req struct {
		ID string `json:"id"`
	}
	json.Unmarshal(msg.Payload, &req)
	if _, ok := buffers[req.ID]; !ok {
		return errorResponse(msg, "buffer not found")
	}
	subscribers[req.ID] = append(subscribers[req.ID], msg.Sender)
	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-buffer-manager",
		Target: msg.Sender,
		Payload: mustMarshal(map[string]string{
			"status": "subscribed",
		}),
	}
}

func handleCreate(msg wasm.Message) wasm.Message {
	var req struct {
		Name         string                 `json:"name"`
		Type         string                 `json:"type"`
		MimeType     string                 `json:"mime_type"`
		BaseBufferID string                 `json:"base_buffer_id"`
		Content      []byte                 `json:"content"`
		Metadata     map[string]interface{} `json:"metadata"`
	}
	json.Unmarshal(msg.Payload, &req)

	id := fmt.Sprintf("buf-%d", nextID)
	nextID++

	b := &Buffer{
		ID:           id,
		Name:         req.Name,
		Type:         req.Type,
		MimeType:     req.MimeType,
		BaseBufferID: req.BaseBufferID,
		Data:         req.Content,
		Timestamp:    time.Now().Unix(),
		Metadata:     req.Metadata,
	}
	if b.Metadata == nil {
		b.Metadata = make(map[string]interface{})
	}
	if b.Type == "" {
		b.Type = "ephemeral"
	}
	buffers[id] = b

	return wasm.Message{
		ID:      msg.ID + "-resp",
		Type:    "response",
		Sender:  "plugin-buffer-manager",
		Target:  msg.Sender,
		Payload: mustMarshal(b),
	}
}

func handleRead(msg wasm.Message) wasm.Message {
	var req struct {
		ID string `json:"id"`
	}
	json.Unmarshal(msg.Payload, &req)

	root, ok := findRootBuffer(req.ID)
	if !ok {
		return errorResponse(msg, "not found")
	}

	type readResponse struct {
		ID      string `json:"id"`
		RootID  string `json:"root_id"`
		Content []byte `json:"content"`
		Size    int    `json:"size"`
	}
	
	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-buffer-manager",
		Target: msg.Sender,
		Payload: mustMarshal(readResponse{
			ID:      req.ID,
			RootID:  root.ID,
			Content: root.Data,
			Size:    len(root.Data),
		}),
	}
}

func handleWrite(msg wasm.Message) wasm.Message {
	var req struct {
		ID      string `json:"id"`
		Content []byte `json:"content"`
		Offset  *int   `json:"offset"`
	}
	json.Unmarshal(msg.Payload, &req)
	if _, ok := buffers[req.ID]; !ok {
		return errorResponse(msg, "not found")
	}
	root, _ := findRootBuffer(req.ID)

	if req.Offset != nil && *req.Offset >= 0 {
		offset := *req.Offset
		if offset+len(req.Content) > len(root.Data) {
			newData := make([]byte, offset+len(req.Content))
			for i := 0; i < len(root.Data); i++ {
				newData[i] = root.Data[i]
			}
			root.Data = newData
		}
		for i := 0; i < len(req.Content); i++ {
			root.Data[offset+i] = req.Content[i]
		}
	} else {
		root.Data = req.Content
	}

	root.Timestamp = time.Now().Unix()
	notifyAll(req.ID, "update")
	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-buffer-manager",
		Target: msg.Sender,
		Payload: mustMarshal(map[string]string{
			"status": "ok",
		}),
	}
}

func handleAppend(msg wasm.Message) wasm.Message {
	var req struct {
		ID      string `json:"id"`
		Content []byte `json:"content"`
	}
	json.Unmarshal(msg.Payload, &req)
	if _, ok := buffers[req.ID]; !ok {
		return errorResponse(msg, "not found")
	}
	root, _ := findRootBuffer(req.ID)

	// Stream pruning logic
	maxHistory := 0
	if val, ok := root.Metadata["max_history"].(float64); ok {
		maxHistory = int(val)
	}

	root.Data = append(root.Data, req.Content...)
	if maxHistory > 0 && len(root.Data) > maxHistory {
		root.Data = root.Data[len(root.Data)-maxHistory:]
	}

	root.Timestamp = time.Now().Unix()
	notifyAll(req.ID, "append")
	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-buffer-manager",
		Target: msg.Sender,
		Payload: mustMarshal(map[string]string{
			"status": "ok",
		}),
	}
}

func handleList(msg wasm.Message) wasm.Message {
	list := make([]*Buffer, 0, len(buffers))
	for _, b := range buffers {
		list = append(list, b)
	}
	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-buffer-manager",
		Target: msg.Sender,
		Payload: mustMarshal(map[string]interface{}{
			"buffers": list,
		}),
	}
}

func handleDelete(msg wasm.Message) wasm.Message {
	var req struct {
		ID string `json:"id"`
	}
	json.Unmarshal(msg.Payload, &req)
	delete(buffers, req.ID)
	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-buffer-manager",
		Target: msg.Sender,
		Payload: mustMarshal(map[string]string{
			"status": "deleted",
		}),
	}
}

func handleClear(msg wasm.Message) wasm.Message {
	var req struct {
		ID string `json:"id"`
	}
	json.Unmarshal(msg.Payload, &req)
	root, ok := findRootBuffer(req.ID)
	if !ok {
		return errorResponse(msg, "not found")
	}
	root.Data = []byte{}
	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-buffer-manager",
		Target: msg.Sender,
		Payload: mustMarshal(map[string]string{
			"status": "ok",
		}),
	}
}

func handleSetMetadata(msg wasm.Message) wasm.Message {
	var req struct {
		ID       string                 `json:"id"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	json.Unmarshal(msg.Payload, &req)
	b, ok := buffers[req.ID]
	if !ok {
		return errorResponse(msg, "not found")
	}
	if b.Metadata == nil {
		b.Metadata = make(map[string]interface{})
	}
	for k, v := range req.Metadata {
		b.Metadata[k] = v
	}
	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-buffer-manager",
		Target: msg.Sender,
		Payload: mustMarshal(map[string]string{
			"status": "ok",
		}),
	}
}

func handleSave(msg wasm.Message) wasm.Message {
	var req struct {
		ID string `json:"id"`
	}
	json.Unmarshal(msg.Payload, &req)
	b, ok := buffers[req.ID]
	if !ok {
		return errorResponse(msg, "not found")
	}

	// Persist metadata using synchronous KV host functions
	metaKey := fmt.Sprintf("buffer:%s:meta", b.ID)
	wasm.KVSet(metaKey, mustMarshal(b))

	// Persist content (only for root buffers)
	root, _ := findRootBuffer(b.ID)
	if root != nil && root.ID == b.ID {
		contentKey := fmt.Sprintf("buffer:%s:content", b.ID)
		wasm.KVSet(contentKey, root.Data)
	}

	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-buffer-manager",
		Target: msg.Sender,
		Payload: mustMarshal(map[string]string{
			"status": "ok",
		}),
	}
}

func handleLoad(msg wasm.Message) wasm.Message {
	// Since we don't have a synchronous KV 'list' host function yet,
	// we still use the async RouteMessage approach for discovery.
	wasm.RouteMessage(wasm.Message{
		ID:      "kv-load-list",
		Type:    "request",
		Sender:  "plugin-buffer-manager",
		Target:  "plugin-kv",
		Method:  "list",
		Payload: mustMarshal(map[string]string{"prefix": "buffer:"}),
	})

	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-buffer-manager",
		Target: msg.Sender,
		Payload: mustMarshal(map[string]string{
			"status": "ok",
		}),
	}
}

func handleKVResponse(msg wasm.Message) wasm.Message {
	wasm.Log("Received KV response: " + msg.ID)
	// Handle list result
	if msg.ID == "kv-load-list-resp" {
		var resp struct {
			Keys []string `json:"keys"`
		}
		json.Unmarshal(msg.Payload, &resp)
		wasm.Log(fmt.Sprintf("Found %d keys", len(resp.Keys)))
		for _, key := range resp.Keys {
			if strings.HasSuffix(key, ":meta") {
				wasm.Log("Loading meta for " + key)
				val := wasm.KVGet(key)
				if val != nil {
					var b Buffer
					if err := json.Unmarshal(val, &b); err == nil {
						buffers[b.ID] = &b
						wasm.Log("Restored buffer " + b.ID)
						// If root, get content too
						if b.BaseBufferID == "" {
							contentKey := fmt.Sprintf("buffer:%s:content", b.ID)
							content := wasm.KVGet(contentKey)
							if content != nil {
								b.Data = content
								wasm.Log(fmt.Sprintf("Restored content for %s: %d bytes", b.ID, len(content)))
							}
						}
					} else {
						wasm.Log("Failed to unmarshal meta: " + key)
					}
				}
			}
		}
	}
	return wasm.Message{Type: "ignore"}
}

func notifyAll(id string, event string) {
	evt := map[string]interface{}{
		"buffer_id": id,
		"event":     event,
		"timestamp": time.Now().Unix(),
	}
	payload := mustMarshal(evt)
	tid := fmt.Sprint(time.Now().UnixNano())
	wasm.RouteMessage(wasm.Message{
		ID:      "evt-pub-" + tid,
		Type:    "event",
		Sender:  "plugin-buffer-manager",
		Target:  "plugin-events",
		Method:  "publish",
		Payload: payload,
	})
	for _, sub := range subscribers[id] {
		wasm.RouteMessage(wasm.Message{
			ID:      "evt-sub-" + tid,
			Type:    "event",
			Sender:  "plugin-buffer-manager",
			Target:  sub,
			Method:  "update",
			Payload: payload,
		})
	}
}

func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

func errorResponse(msg wasm.Message, err string) wasm.Message {
	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-buffer-manager",
		Target: msg.Sender,
		Payload: mustMarshal(map[string]string{
			"error": err,
		}),
	}
}
