//go:build wasip1 || wasm

package main

import (
	. "github.com/jnesbitt/alloy-go/pkg/wasm/bindings/guest"
	. "github.com/jnesbitt/alloy-go/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"strings"
	"time"

)

// Buffer represents a data buffer.
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
	plugin      *Plugin
)

func main() {
	// Create a new WIT-based plugin
	plugin = NewPlugin("buffer-manager").
		WithMetadata(
			"Buffer Manager", 
			"Manages data buffers for the system",
			"0.1.0", 
			"Alloy Team",
		).
		WithTags("buffer", "data", "storage").
		WithCapability("create", "Create a new buffer").
		WithCapability("list", "List all buffers").
		WithCapability("read", "Read buffer content").
		WithCapability("write", "Write buffer content").
		WithCapability("delete", "Delete a buffer").
		WithCapability("save", "Save buffer to persistent storage").
		WithCapability("load", "Load buffers from persistent storage")

	// Set up message handlers
	plugin.Handle("create", handleCreate)
	plugin.Handle("read", handleRead)
	plugin.Handle("write", handleWrite)
	plugin.Handle("append", handleAppend)
	plugin.Handle("list", handleList)
	plugin.Handle("delete", handleDelete)
	plugin.Handle("subscribe", handleSubscribe)
	plugin.Handle("clear", handleClear)
	plugin.Handle("save", handleSave)
	plugin.Handle("load", handleLoad)
	plugin.Handle("set_metadata", handleSetMetadata)

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "Buffer manager initializing")
		return nil
	})

	// Run the plugin
	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

// findRootBuffer finds the root buffer for a given buffer ID.
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

// handleSubscribe handles buffer subscription requests.
func handleSubscribe(msg AlloyMessage) AlloyMessage {
	var req struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	if _, ok := buffers[req.Id]; !ok {
		return plugin.ErrorReply(msg, "buffer_not_found")
	}

	subscribers[req.Id] = append(subscribers[req.Id], msg.Sender)

	return plugin.Reply(msg, map[string]string{
		"status": "subscribed",
	})
}

// handleCreate handles buffer creation requests.
func handleCreate(msg AlloyMessage) AlloyMessage {
	var req struct {
		Name         string                 `json:"name"`
		Type         string                 `json:"type"`
		MimeType     string                 `json:"mime_type"`
		BaseBufferID string                 `json:"base_buffer_id"`
		Content      []byte                 `json:"content"`
		Metadata     map[string]interface{} `json:"metadata"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	id := fmt.Sprintf("buf-%d", nextID)
	nextID++

	b := &Buffer{
		Id:           id,
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

	// Notify subscribers
	notifyAll(id, "create")

	return plugin.Reply(msg, b)
}

// handleRead handles buffer read requests.
func handleRead(msg AlloyMessage) AlloyMessage {
	var req struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	root, ok := findRootBuffer(req.Id)
	if resp.Id == "" {
		return plugin.ErrorReply(msg, "not_found")
	}

	type readResponse struct {
		ID      string `json:"id"`
		RootID  string `json:"root_id"`
		Content []byte `json:"content"`
		Size    int    `json:"size"`
	}

	return plugin.Reply(msg, readResponse{
		Id:      req.Id,
		RootID:  root.Id,
		Content: root.Data,
		Size:    len(root.Data),
	})
}

// handleWrite handles buffer write requests.
func handleWrite(msg AlloyMessage) AlloyMessage {
	var req struct {
		ID      string `json:"id"`
		Content []byte `json:"content"`
		Offset  *int   `json:"offset"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	if _, ok := buffers[req.Id]; !ok {
		return plugin.ErrorReply(msg, "not_found")
	}
	root, _ := findRootBuffer(req.Id)

	if req.Offset != nil && *req.Offset >= 0 {
		offset := *req.Offset
		if offset+len(req.Content) > len(root.Data) {
			newData := make([]byte, offset+len(req.Content))
			copy(newData, root.Data)
			root.Data = newData
		}
		for i := 0; i < len(req.Content); i++ {
			root.Data[offset+i] = req.Content[i]
		}
	} else {
		root.Data = req.Content
	}

	root.Timestamp = time.Now().Unix()
	notifyAll(req.Id, "update")

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
	})
}

// handleAppend handles buffer append requests.
func handleAppend(msg AlloyMessage) AlloyMessage {
	var req struct {
		ID      string `json:"id"`
		Content []byte `json:"content"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	if _, ok := buffers[req.Id]; !ok {
		return plugin.ErrorReply(msg, "not_found")
	}
	root, _ := findRootBuffer(req.Id)

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
	notifyAll(req.Id, "append")

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
	})
}

// handleList handles buffer list requests.
func handleList(msg AlloyMessage) AlloyMessage {
	list := make([]*Buffer, 0, len(buffers))
	for _, b := range buffers {
		list = append(list, b)
	}

	return plugin.Reply(msg, map[string]interface{}{
		"buffers": list,
	})
}

// handleDelete handles buffer deletion requests.
func handleDelete(msg AlloyMessage) AlloyMessage {
	var req struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	delete(buffers, req.Id)

	// Also delete from persistent storage
	plugin.KVDelete(fmt.Sprintf("buffer:%s:meta", req.Id))
	plugin.KVDelete(fmt.Sprintf("buffer:%s:content", req.Id))

	// Notify subscribers
	notifyAll(req.Id, "delete")

	return plugin.Reply(msg, map[string]string{
		"status": "deleted",
	})
}

// handleClear handles buffer clear requests.
func handleClear(msg AlloyMessage) AlloyMessage {
	var req struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	root, ok := findRootBuffer(req.Id)
	if resp.Id == "" {
		return plugin.ErrorReply(msg, "not_found")
	}

	root.Data = []byte{}
	notifyAll(req.Id, "clear")

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
	})
}

// handleSetMetadata handles metadata update requests.
func handleSetMetadata(msg AlloyMessage) AlloyMessage {
	var req struct {
		ID       string                 `json:"id"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	b, ok := buffers[req.Id]
	if resp.Id == "" {
		return plugin.ErrorReply(msg, "not_found")
	}

	if b.Metadata == nil {
		b.Metadata = make(map[string]interface{})
	}
	for k, v := range req.Metadata {
		b.Metadata[k] = v
	}

	notifyAll(req.Id, "metadata_update")

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
	})
}

// handleSave handles buffer save requests.
func handleSave(msg AlloyMessage) AlloyMessage {
	var req struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	b, ok := buffers[req.Id]
	if resp.Id == "" {
		return plugin.ErrorReply(msg, "not_found")
	}

	// Persist metadata
	metaKey := fmt.Sprintf("buffer:%s:meta", b.Id)
	metaData, _ := json.Marshal(b)
	plugin.KVSet(metaKey, metaData)

	// Persist content (only for root buffers)
	root, _ := findRootBuffer(b.Id)
	if root != nil && root.Id == b.Id {
		contentKey := fmt.Sprintf("buffer:%s:content", b.Id)
		plugin.KVSet(contentKey, root.Data)
	}

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
	})
}

// handleLoad handles buffer load requests.
func handleLoad(msg AlloyMessage) AlloyMessage {
	keys, _ := plugin.KVList("buffer:")
	plugin.Log("info", fmt.Sprintf("Loading: found %d keys", len(keys)))

	loadedCount := 0
	for _, key := range keys {
		if strings.HasSuffix(key, ":meta") {
			val, _ := plugin.KVGet(key)
			if val != nil {
				var b Buffer
				if err := json.Unmarshal(val, &b); err == nil {
					buffers[b.Id] = &b
					plugin.Log("info", "Restored buffer " + b.Id)
					loadedCount++
					// If root, get content too
					if b.BaseBufferID == "" {
						contentKey := fmt.Sprintf("buffer:%s:content", b.Id)
						content, _ := plugin.KVGet(contentKey)
						if content != nil {
							b.Data = content
						}
					}
				}
			}
		}
	}

	plugin.Log("info", fmt.Sprintf("Loaded %d buffers", loadedCount))

	// Notify about loaded buffers
	for id := range buffers {
		notifyAll(id, "load")
	}

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
		"count": fmt.Sprintf("%d", loadedCount),
	})
}

// notifyAll notifies all subscribers about a buffer event.
func notifyAll(id string, event string) {
	evt := map[string]interface{}{
		"buffer_id": id,
		"event":     event,
		"timestamp": time.Now().Unix(),
	}
	payload, _ := json.Marshal(evt)
	tid := fmt.Sprint(time.Now().UnixNano())

	// Notify events plugin
	plugin.RouteMessage(AlloyMessage{
		Id:      "evt-pub-" + tid,
		Method:  "publish",
		Sender:  "buffer-manager",
		Target:  Some("plugin-events"),
		Payload: payload,
	})

	// Notify subscribers
	for _, sub := range subscribers[id] {
		plugin.RouteMessage(AlloyMessage{
			Id:      "evt-sub-" + tid,
			Method:  "update",
			Sender:  "buffer-manager",
			Target:  Some(sub),
			Payload: payload,
		})
	}
}