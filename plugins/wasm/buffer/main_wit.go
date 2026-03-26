//go:build wasip1 || wasm

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/james-nesbitt/alloy/pkg/wasm/guest"
)

// Operation represents a buffer modification.
type Operation struct {
	Version int    `json:"v"`
	Offset  int    `json:"o"`
	Length  int    `json:"l"`
	Type    string `json:"t"` // "insert", "delete", "replace"
}

// Buffer represents a data buffer internal to this plugin.
type Buffer struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"` // ephemeral, file, stream
	MimeType     string                 `json:"mime_type"`
	Source       string                 `json:"source,omitempty"`
	ReadOnly     bool                   `json:"read_only"`
	BaseBufferID string                 `json:"base_buffer_id,omitempty"`
	Version      int                    `json:"version"`
	Metadata     map[string]interface{} `json:"metadata"`
	Data         []byte                 `json:"-"`
	Timestamp    int64                  `json:"timestamp"`
	UserCursors  map[string]Cursor      `json:"user_cursors,omitempty"`
	History      []Operation            `json:"-"`
}

type Cursor struct {
	Row      int    `json:"row"`
	Col      int    `json:"col"`
	User     string `json:"user"`
	LastSeen int64  `json:"last_seen"`
}

var (
	buffers     = make(map[string]*Buffer)
	subscribers = make(map[string][]string)
	nextID      = 1
	plugin      *guest.Plugin
)

func main() {
	// Create a new WIT-based plugin
	plugin = guest.NewPlugin("buffer").
		WithMetadata(
			"Buffer Manager", 
			"Manages data buffers for the system",
			"0.1.0", 
			"Alloy Team",
		).
		WithTags("buffer", "data", "storage").
		WithCapability("buffer:create", "Create a new buffer").WithShortcut("b n").
		WithCapability("buffer:list", "List all buffers").WithShortcut("b l").
		WithCapability("buffer:read", "Read buffer content").WithShortcut("b r").
		WithCapability("buffer:write", "Write buffer content").WithShortcut("b w").
		WithCapability("buffer:delete", "Delete a buffer").WithShortcut("b d").
		WithCapability("buffer:unload", "Remove a buffer from memory only").
		WithCapability("buffer:save", "Save buffer to persistent storage").WithShortcut("b s").
		WithCapability("buffer:load", "Load buffers from persistent storage").WithShortcut("b o").
		WithCapability("buffer:update-cursor", "Update user cursor position").
		WithCapability("ui:view:editor", "Open the code editor view").
		WithAnnotations("ui:view:editor", map[string]string{"type": "editor", "title": "Code Editor"})

	// Set up message handlers
	plugin.Handle("buffer:create", handleCreate)
	plugin.Handle("buffer:read", handleRead)
	plugin.Handle("buffer:write", handleWrite)
	plugin.Handle("buffer:append", handleAppend)
	plugin.Handle("buffer:list", handleList)
	plugin.Handle("buffer:delete", handleDelete)
	plugin.Handle("buffer:unload", handleUnload)
	plugin.Handle("buffer:subscribe", handleSubscribe)
	plugin.Handle("buffer:clear", handleClear)
	plugin.Handle("buffer:save", handleSave)
	plugin.Handle("buffer:load", handleLoad)
	plugin.Handle("buffer:set_metadata", handleSetMetadata)
	plugin.Handle("buffer:update_cursor", handleUpdateCursor)
	plugin.Handle("ui:view:editor", func(msg guest.AlloyMessage) guest.AlloyMessage { return plugin.Reply(msg, "ok") })

	// Backward compatibility handlers
	plugin.Handle("create", handleCreate)
	plugin.Handle("read", handleRead)
	plugin.Handle("write", handleWrite)
	plugin.Handle("append", handleAppend)
	plugin.Handle("list", handleList)
	plugin.Handle("update_cursor", handleUpdateCursor)
	plugin.Handle("clear", handleClear)
	plugin.Handle("save", handleSave)
	plugin.Handle("load", handleLoad)
	plugin.Handle("delete", handleDelete)
	plugin.Handle("unload", handleUnload)
	plugin.Handle("subscribe", handleSubscribe)
	plugin.Handle("set_metadata", handleSetMetadata)

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "Buffer manager initializing")
		
		// Register in the component registry
		plugin.RegisterCapability(guest.AlloyCapability{
			Method:      "buffer:read",
			Description: "Directly read buffer content",
		})
		plugin.RegisterCapability(guest.AlloyCapability{
			Method:      "buffer:write",
			Description: "Directly write buffer content",
		})

		// Register a dashboard widget
		plugin.RegisterWidget(guest.AlloyWidget{
			Id:                "buffer-summary",
			Title:             "Active Buffers",
			ContentType:       "text",
			Content:           []byte("No active buffers"),
			RefreshIntervalMs: 5000,
		})

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
func handleSubscribe(msg guest.AlloyMessage) guest.AlloyMessage {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	if _, ok := buffers[req.ID]; !ok {
		return plugin.ErrorReply(msg, "buffer_not_found")
	}

	subscribers[req.ID] = append(subscribers[req.ID], msg.Sender)

	return plugin.Reply(msg, map[string]string{
		"status": "subscribed",
	})
}

// handleCreate handles buffer creation requests.
func handleCreate(msg guest.AlloyMessage) guest.AlloyMessage {
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
		ID:           id,
		Name:         req.Name,
		Type:         req.Type,
		MimeType:     req.MimeType,
		BaseBufferID: req.BaseBufferID,
		Data:         req.Content,
		Version:      0, // Start from 0
		Timestamp:    time.Now().Unix(),
		Metadata:     req.Metadata,
		History:      make([]Operation, 0),
	}
	if b.Metadata == nil {
		b.Metadata = make(map[string]interface{})
	}
	if b.Type == "" {
		b.Type = "ephemeral"
	}
	buffers[id] = b

	// Create in Host if large/shared candidate
	if len(req.Content) > 0 {
		plugin.WriteBuffer(id, req.Content)
	}

	// Notify subscribers
	notifyAll(id, "create")

	return plugin.Reply(msg, b)
}

// handleUpdateCursor updates the cursor position for a user.
func handleUpdateCursor(msg guest.AlloyMessage) guest.AlloyMessage {
	var req struct {
		ID  string `json:"id"`
		Row int    `json:"row"`
		Col int    `json:"col"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	b, ok := buffers[req.ID]
	if !ok {
		return plugin.ErrorReply(msg, "buffer_not_found")
	}

	if b.UserCursors == nil {
		b.UserCursors = make(map[string]Cursor)
	}

	actor := msg.Sender // Defaulting to sender ID for now
	b.UserCursors[actor] = Cursor{
		Row:      req.Row,
		Col:      req.Col,
		User:     actor,
		LastSeen: time.Now().Unix(),
	}

	// Sync presence to host-side semantic presence buffer
	presenceID := "presence:" + req.ID
	presenceData, _ := json.Marshal(b.UserCursors)
	plugin.WriteBuffer(presenceID, presenceData)

	notifyAll(req.ID, "cursor_update")

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
	})
}

// handleRead handles buffer read requests.
func handleRead(msg guest.AlloyMessage) guest.AlloyMessage {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	// Try Direct Host Path first
	if b, ok := plugin.ReadBuffer(req.ID); ok {
		return plugin.Reply(msg, b)
	}

	// Fallback to internal tracker
	root, ok := findRootBuffer(req.ID)
	if !ok {
		return plugin.ErrorReply(msg, "not_found")
	}

	return plugin.Reply(msg, guest.AlloyBuffer{
		Id:           root.ID,
		Name:         root.Name,
		Content:      root.Data,
		LastModified: uint64(root.Timestamp),
		MimeType:     root.MimeType,
	})
}

// handleWrite handles buffer write requests.
func handleWrite(msg guest.AlloyMessage) guest.AlloyMessage {
	plugin.Log("info", "handleWrite received message")
	var req struct {
		ID          string `json:"id"`
		BaseVersion int    `json:"base_version"`
		Content     []byte `json:"content"`
		Offset      *int   `json:"offset"`
		Action      string `json:"action"` // "insert", "delete", "replace"
		Force       bool   `json:"force"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		plugin.Log("error", "Failed to unmarshal write request: "+err.Error())
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	root, ok := findRootBuffer(req.ID)
	if !ok {
		plugin.Log("error", "Buffer not found: "+req.ID)
		return plugin.ErrorReply(msg, "not_found")
	}

	if req.Action == "" {
		req.Action = "replace"
	}

	targetOffset := 0
	if req.Offset != nil {
		targetOffset = *req.Offset
	}

	if !req.Force && req.BaseVersion != root.Version {
		if req.Offset == nil {
			return plugin.ErrorReply(msg, "conflict_detected")
		}

		if req.BaseVersion < root.Version && len(root.History) > 0 {
			historyStartIdx := -1
			for i, op := range root.History {
				if op.Version >= req.BaseVersion {
					historyStartIdx = i
					break
				}
			}

			if historyStartIdx != -1 {
				for i := historyStartIdx; i < len(root.History); i++ {
					op := root.History[i]
					if op.Type == "insert" {
						if op.Offset <= targetOffset {
							targetOffset += op.Length
						}
					} else if op.Type == "delete" {
						if op.Offset < targetOffset {
							if op.Offset + op.Length <= targetOffset {
								targetOffset -= op.Length
							} else {
								targetOffset = op.Offset
							}
						}
					}
				}
			} else {
				return plugin.ErrorReply(msg, "conflict_detected_history_lost")
			}
		} else if req.BaseVersion < root.Version {
			return plugin.ErrorReply(msg, "conflict_detected")
		}
	}

	if targetOffset < 0 {
		targetOffset = 0
	}
	if targetOffset > len(root.Data) {
		targetOffset = len(root.Data)
	}

	op := Operation{
		Version: root.Version,
		Offset:  targetOffset,
		Length:  len(req.Content),
		Type:    req.Action,
	}

	if req.Offset != nil && *req.Offset >= 0 {
		if req.Action == "insert" {
			newData := make([]byte, len(root.Data)+len(req.Content))
			copy(newData, root.Data[:targetOffset])
			copy(newData[targetOffset:], req.Content)
			copy(newData[targetOffset+len(req.Content):], root.Data[targetOffset:])
			root.Data = newData
		} else if req.Action == "delete" {
			endOffset := targetOffset + len(req.Content)
			if endOffset > len(root.Data) {
				endOffset = len(root.Data)
			}
			newData := make([]byte, len(root.Data)-(endOffset-targetOffset))
			copy(newData, root.Data[:targetOffset])
			copy(newData[targetOffset:], root.Data[endOffset:])
			root.Data = newData
			op.Length = endOffset - targetOffset
		} else { // "replace"
			if targetOffset+len(req.Content) > len(root.Data) {
				newData := make([]byte, targetOffset+len(req.Content))
				copy(newData, root.Data)
				root.Data = newData
			}
			for i := 0; i < len(req.Content); i++ {
				root.Data[targetOffset+i] = req.Content[i]
			}
		}
	} else {
		root.Data = req.Content
		op.Offset = 0
		op.Type = "replace"
		op.Length = len(req.Content)
	}

	root.Version++
	root.Timestamp = time.Now().Unix()
	
	root.History = append(root.History, op)
	if len(root.History) > 100 {
		root.History = root.History[len(root.History)-100:]
	}

	// Sync with Host
	plugin.WriteBuffer(req.ID, root.Data)

	notifyAll(req.ID, "update")

	return plugin.Reply(msg, map[string]interface{}{
		"status":  "ok",
		"version": root.Version,
	})
}

// handleAppend handles buffer append requests.
func handleAppend(msg guest.AlloyMessage) guest.AlloyMessage {
	var req struct {
		ID      string `json:"id"`
		Content []byte `json:"content"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	root, ok := findRootBuffer(req.ID)
	if !ok {
		return plugin.ErrorReply(msg, "not_found")
	}

	root.Data = append(root.Data, req.Content...)
	
	// Enforce max_history if set (sliding window for streams)
	if root.Metadata != nil {
		if val, exists := root.Metadata["max_history"]; exists {
			var limit int
			switch v := val.(type) {
			case float64:
				limit = int(v)
			case int:
				limit = v
			}
			if limit > 0 && len(root.Data) > limit {
				root.Data = root.Data[len(root.Data)-limit:]
			}
		}
	}

	root.Timestamp = time.Now().Unix()

	// Sync with Host
	plugin.WriteBuffer(req.ID, root.Data)

	notifyAll(req.ID, "append")

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
	})
}

// handleList handles buffer list requests.
func handleList(msg guest.AlloyMessage) guest.AlloyMessage {
	list := make([]*Buffer, 0, len(buffers))
	for _, b := range buffers {
		list = append(list, b)
	}

	return plugin.Reply(msg, map[string]interface{}{
		"buffers": list,
	})
}

// handleDelete handles buffer deletion requests.
func handleDelete(msg guest.AlloyMessage) guest.AlloyMessage {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	delete(buffers, req.ID)
	plugin.KVDelete(fmt.Sprintf("buffer:%s:meta", req.ID))
	plugin.KVDelete(fmt.Sprintf("buffer:%s:content", req.ID))
	notifyAll(req.ID, "delete")

	return plugin.Reply(msg, map[string]string{
		"status": "deleted",
	})
}

// handleUnload removes a buffer from memory only.
func handleUnload(msg guest.AlloyMessage) guest.AlloyMessage {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	delete(buffers, req.ID)
	notifyAll(req.ID, "unload")

	return plugin.Reply(msg, map[string]string{
		"status": "unloaded",
	})
}

// handleClear handles buffer clear requests.
func handleClear(msg guest.AlloyMessage) guest.AlloyMessage {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	root, ok := findRootBuffer(req.ID)
	if !ok {
		return plugin.ErrorReply(msg, "not_found")
	}

	root.Data = []byte{}

	// Sync with Host
	plugin.WriteBuffer(req.ID, root.Data)

	notifyAll(req.ID, "clear")

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
	})
}

// handleSetMetadata handles metadata update requests.
func handleSetMetadata(msg guest.AlloyMessage) guest.AlloyMessage {
	var req struct {
		ID       string                 `json:"id"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	b, ok := buffers[req.ID]
	if !ok {
		return plugin.ErrorReply(msg, "not_found")
	}

	if b.Metadata == nil {
		b.Metadata = make(map[string]interface{})
	}
	for k, v := range req.Metadata {
		b.Metadata[k] = v
	}

	notifyAll(req.ID, "metadata_update")

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
	})
}

// handleSave handles buffer save requests.
func handleSave(msg guest.AlloyMessage) guest.AlloyMessage {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	b, ok := buffers[req.ID]
	if !ok {
		return plugin.ErrorReply(msg, "not_found")
	}

	metaKey := fmt.Sprintf("buffer:%s:meta", b.ID)
	metaData, _ := json.Marshal(b)
	plugin.KVSet(metaKey, metaData)

	root, rootOk := findRootBuffer(b.ID)
	if rootOk && root.ID == b.ID {
		contentKey := fmt.Sprintf("buffer:%s:content", b.ID)
		plugin.KVSet(contentKey, root.Data)
	}

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
	})
}

// handleLoad handles buffer load requests.
func handleLoad(msg guest.AlloyMessage) guest.AlloyMessage {
	keys := plugin.KVList("buffer:")
	loadedCount := 0
	for _, key := range keys {
		if strings.HasSuffix(key, ":meta") {
			val, ok := plugin.KVGet(key)
			if ok && val != nil {
				var b Buffer
				if err := json.Unmarshal(val, &b); err == nil {
					buffers[b.ID] = &b
					loadedCount++
					if b.BaseBufferID == "" {
						contentKey := fmt.Sprintf("buffer:%s:content", b.ID)
						content, contentOk := plugin.KVGet(contentKey)
						if contentOk && content != nil {
							b.Data = content
						}
					}
				}
			}
		}
	}

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
	evtData, _ := json.Marshal(evt)
	
	topic := "buffer:update"
	if event == "cursor_update" {
		topic = "buffer:cursors_updated"
	}
	
	publishPayload, _ := json.Marshal(map[string]any{
		"topic": topic,
		"data":  json.RawMessage(evtData),
	})

	plugin.RouteMessage(guest.AlloyMessage{
		Id:      "evt-pub-" + id,
		MsgType: "request",
		Method:  "publish",
		Sender:  "buffer",
		Target:  guest.Some("events"),
		Payload: publishPayload,
	})

	// Also notify internal subscribers directly
	for _, sub := range subscribers[id] {
		plugin.RouteMessage(guest.AlloyMessage{
			Id:      fmt.Sprintf("evt-%s-%s-%d", id, event, time.Now().UnixNano()),
			MsgType: "event",
			Method:  "update",
			Sender:  "buffer",
			Target:  guest.Some(sub),
			Payload: evtData,
		})
	}
}

// Direct interaction handlers (WIT interface implementation)

func ReadBuffer(id string) (guest.AlloyBuffer, bool) {
	root, ok := findRootBuffer(id)
	if !ok {
		return guest.AlloyBuffer{}, false
	}
	return guest.AlloyBuffer{
		Id:           root.ID,
		Name:         root.Name,
		Content:      root.Data,
		LastModified: uint64(root.Timestamp),
		MimeType:     root.MimeType,
	}, true
}

func WriteBuffer(id string, content []byte) bool {
	root, ok := findRootBuffer(id)
	if !ok {
		return false
	}
	root.Data = content
	root.Version++
	root.Timestamp = time.Now().Unix()
	notifyAll(id, "update")
	return true
}

func ListBuffers() []string {
	ids := make([]string, 0, len(buffers))
	for id := range buffers {
		ids = append(ids, id)
	}
	return ids
}
