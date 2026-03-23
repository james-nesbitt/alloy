//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/build/gen/bindings/guest"
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Operation represents a buffer modification.
type Operation struct {
	Version int    `json:"v"`
	Offset  int    `json:"o"`
	Length  int    `json:"l"`
	Type    string `json:"t"` // "insert", "delete", "replace"
}

// Buffer represents a data buffer.
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
	plugin      *Plugin
)

func main() {
	// Create a new WIT-based plugin
	plugin = NewPlugin("buffer").
		WithMetadata(
			"Buffer Manager", 
			"Manages data buffers for the system",
			"0.1.0", 
			"Alloy Team",
		).
		WithTags("buffer", "data", "storage").
		WithCapability("create", "Create a new buffer").WithShortcut("b n").
		WithCapability("list", "List all buffers").WithShortcut("b l").
		WithCapability("read", "Read buffer content").WithShortcut("b r").
		WithCapability("write", "Write buffer content").WithShortcut("b w").
		WithCapability("delete", "Delete a buffer").WithShortcut("b d").
		WithCapability("unload", "Remove a buffer from memory only").
		WithCapability("save", "Save buffer to persistent storage").WithShortcut("b s").
		WithCapability("load", "Load buffers from persistent storage").WithShortcut("b o")

	// Set up message handlers
	plugin.Handle("create", handleCreate)
	plugin.Handle("read", handleRead)
	plugin.Handle("write", handleWrite)
	plugin.Handle("append", handleAppend)
	plugin.Handle("list", handleList)
	plugin.Handle("delete", handleDelete)
	plugin.Handle("unload", handleUnload)
	plugin.Handle("subscribe", handleSubscribe)
	plugin.Handle("clear", handleClear)
	plugin.Handle("save", handleSave)
	plugin.Handle("load", handleLoad)
	plugin.Handle("set_metadata", handleSetMetadata)
	plugin.Handle("update_cursor", handleUpdateCursor)

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "Buffer manager initializing")
		
		// Register in the component registry
		plugin.RegisterCapability(AlloyCapability{
			Method:      "buffer-manager:read",
			Description: "Directly read buffer content",
		})
		plugin.RegisterCapability(AlloyCapability{
			Method:      "buffer-manager:write",
			Description: "Directly write buffer content",
		})

		// Register a dashboard widget
		plugin.RegisterWidget(AlloyWidget{
			Id:                "buffer-summary",
			Title:             "Active Buffers",
			ContentType:       "text",
			Content:           []byte("No active buffers"),
			RefreshIntervalMs: 5000,
		})

		// Periodically clean up stale cursors
		go func() {
			for {
				time.Sleep(10 * time.Second) // Faster update during dev
				now := time.Now().Unix()
				for buffID, b := range buffers {
					if b.UserCursors != nil {
						changed := false
						for userID, cursor := range b.UserCursors {
							if now-cursor.LastSeen > 300 { // 5-minute timeout
								delete(b.UserCursors, userID)
								changed = true
							}
						}
						if changed {
							notifyAll(buffID, "cursors_updated")
						}
					}
				}
				
				// Update Dashboard Widget
				if len(buffers) > 0 {
					var lines []string
					for _, b := range buffers {
						lines = append(lines, fmt.Sprintf("● %s (%s) - %d bytes", b.Name, b.MimeType, len(b.Data)))
					}
					plugin.UpdateWidget("buffer-summary", []byte(strings.Join(lines, "\n")))
				}
			}
		}()
		
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

	// Notify subscribers
	notifyAll(id, "create")

	return plugin.Reply(msg, b)
}

// handleUpdateCursor updates the cursor position for a user.
func handleUpdateCursor(msg AlloyMessage) AlloyMessage {
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

	actor := msg.Sender // Defaulting to sender ID for now; in a real scenario, use authenticated actor ID
	b.UserCursors[actor] = Cursor{
		Row:      req.Row,
		Col:      req.Col,
		User:     actor,
		LastSeen: time.Now().Unix(),
	}

	notifyAll(req.ID, "cursor_update")

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
	})
}

// handleRead handles buffer read requests.
func handleRead(msg AlloyMessage) AlloyMessage {
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

	type readResponse struct {
		ID      string            `json:"id"`
		RootID  string            `json:"root_id"`
		Content []byte            `json:"content"`
		Version int               `json:"version"`
		Cursors map[string]Cursor `json:"cursors,omitempty"`
		Size    int               `json:"size"`
	}

	return plugin.Reply(msg, readResponse{
		ID:      req.ID,
		RootID:  root.ID,
		Content: root.Data,
		Version: root.Version,
		Cursors: root.UserCursors,
		Size:    len(root.Data),
	})
}

// handleWrite handles buffer write requests.
func handleWrite(msg AlloyMessage) AlloyMessage {
	plugin.Log("info", "handleWrite received message")
	var req struct {
		ID          string `json:"id"`
		BaseVersion int    `json:"base_version"`
		Content     []byte `json:"content"`
		Offset      *int   `json:"offset"`
		Action      string `json:"action"` // "insert", "delete", "replace" (default "replace")
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

	plugin.Log("info", fmt.Sprintf("Write: id=%s action=%s offset=%d", req.ID, req.Action, 0))
	if req.Offset != nil {
		plugin.Log("info", fmt.Sprintf("Write: has offset %d", *req.Offset))
	}

	if req.Action == "" {
		req.Action = "replace"
	}

	// Conflict detection & Transformation
	targetOffset := 0
	if req.Offset != nil {
		targetOffset = *req.Offset
	}

	if !req.Force && req.BaseVersion != root.Version {
		// Attempt to transform the operation if we have history
		if req.BaseVersion < root.Version && len(root.History) > 0 {
			plugin.Log("info", fmt.Sprintf("Transforming write: base=%d current=%d", req.BaseVersion, root.Version))
			
			// Find first operation that the user hasn't seen
			// (Operations with Version >= req.BaseVersion)
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
								// Partial overlap - complex resolve: for now, move to start of deletion
								targetOffset = op.Offset
							}
						}
					}
				}
				plugin.Log("debug", fmt.Sprintf("Transformed offset: %d -> %d", *req.Offset, targetOffset))
			} else {
				// History lost - fallback to conflict fail
				plugin.Log("warn", "History too old, cannot transform")
				return plugin.ErrorReply(msg, "conflict_detected_history_lost")
			}
		} else if req.BaseVersion < root.Version {
			return plugin.ErrorReply(msg, "conflict_detected")
		}
	}

	op := Operation{
		Version: root.Version,
		Offset:  targetOffset,
		Length:  len(req.Content),
		Type:    req.Action,
	}

	if req.Offset != nil && *req.Offset >= 0 {
		if req.Action == "insert" {
			// Shift existing data to the right
			newData := make([]byte, len(root.Data)+len(req.Content))
			copy(newData, root.Data[:targetOffset])
			copy(newData[targetOffset:], req.Content)
			copy(newData[targetOffset+len(req.Content):], root.Data[targetOffset:])
			root.Data = newData
		} else if req.Action == "delete" {
			// Remove data
			if targetOffset+len(req.Content) <= len(root.Data) {
				newData := make([]byte, len(root.Data)-len(req.Content))
				copy(newData, root.Data[:targetOffset])
				copy(newData[targetOffset:], root.Data[targetOffset+len(req.Content):])
				root.Data = newData
			}
		} else { // "replace" (default)
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
		// Full replace
		root.Data = req.Content
		op.Offset = 0
		op.Type = "replace"
		op.Length = len(req.Content)
	}

	root.Version++
	root.Timestamp = time.Now().Unix()
	
	// Append to history and truncate
	root.History = append(root.History, op)
	if len(root.History) > 100 {
		root.History = root.History[len(root.History)-100:]
	}

	notifyAll(req.ID, "update")

	return plugin.Reply(msg, map[string]interface{}{
		"status":  "ok",
		"version": root.Version,
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

	root, ok := findRootBuffer(req.ID)
	if !ok {
		return plugin.ErrorReply(msg, "not_found")
	}

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
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "failed_to_unmarshal_request")
	}

	delete(buffers, req.ID)

	// Also delete from persistent storage
	plugin.KVDelete(fmt.Sprintf("buffer:%s:meta", req.ID))
	plugin.KVDelete(fmt.Sprintf("buffer:%s:content", req.ID))

	// Notify subscribers
	notifyAll(req.ID, "delete")

	return plugin.Reply(msg, map[string]string{
		"status": "deleted",
	})
}

// handleUnload removes a buffer from memory without deleting it from disk.
func handleUnload(msg AlloyMessage) AlloyMessage {
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
func handleClear(msg AlloyMessage) AlloyMessage {
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
	notifyAll(req.ID, "clear")

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
func handleSave(msg AlloyMessage) AlloyMessage {
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

	plugin.Log("info", "Saving buffer "+b.ID)

	// Persist metadata
	metaKey := fmt.Sprintf("buffer:%s:meta", b.ID)
	metaData, _ := json.Marshal(b)
	if !plugin.KVSet(metaKey, metaData) {
		return plugin.ErrorReply(msg, "failed_to_save_metadata")
	}

	// Persist content (only for root buffers)
	root, rootOk := findRootBuffer(b.ID)
	if rootOk && root.ID == b.ID {
		contentKey := fmt.Sprintf("buffer:%s:content", b.ID)
		if !plugin.KVSet(contentKey, root.Data) {
			return plugin.ErrorReply(msg, "failed_to_save_content")
		}
	}

	plugin.Log("info", "Saved buffer "+b.ID)

	return plugin.Reply(msg, map[string]string{
		"status": "ok",
	})
}

// handleLoad handles buffer load requests.
func handleLoad(msg AlloyMessage) AlloyMessage {
	keys := plugin.KVList("buffer:")
	plugin.Log("info", fmt.Sprintf("Loading: found %d keys", len(keys)))

	loadedCount := 0
	for _, key := range keys {
		if strings.HasSuffix(key, ":meta") {
			val, ok := plugin.KVGet(key)
			if ok && val != nil {
				var b Buffer
				if err := json.Unmarshal(val, &b); err == nil {
					buffers[b.ID] = &b
					plugin.Log("info", "Restored buffer " + b.ID)
					loadedCount++
					// If root, get content too
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
		MsgType: "request",
		Method:  "publish",
		Sender:  "buffer",
		Target:  Some("events"),
		Payload: payload,
	})

	// Notify subscribers
	for _, sub := range subscribers[id] {
		plugin.RouteMessage(AlloyMessage{
			Id:      "evt-sub-" + tid,
			MsgType: "event",
			Method:  "update",
			Sender:  "buffer",
			Target:  Some(sub),
			Payload: payload,
		})
	}
}

// Direct interaction handlers (WIT interface implementation)

func ReadBuffer(id string) (AlloyBuffer, bool) {
	root, ok := findRootBuffer(id)
	if !ok {
		return AlloyBuffer{}, false
	}
	return AlloyBuffer{
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
