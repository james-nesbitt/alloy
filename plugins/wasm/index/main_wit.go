//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Document represents a semantically indexed file, snippet, or activity.
type Document struct {
	ID        string   `json:"id"`
	Path      string   `json:"path"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	Timestamp int64    `json:"timestamp"`
	Author    string   `json:"author,omitempty"`
	Source    string   `json:"source,omitempty"` // e.g., "buffer", "chat", "task"
}

// SearchRequest represents a semantic search query.
type SearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

// SearchResult represents a document hit.
type SearchResult struct {
	Document Document `json:"document"`
	Score    float64  `json:"score"`
}

var (
	plugin *Plugin
)

const (
	DocPrefix = "idx:doc:"
)

func main() {
	plugin = NewPlugin("index").
		WithMetadata(
			"Knowledge Graph Indexer",
			"Background activity indexing and persistent knowledge graph",
			"0.2.0",
			"Alloy Team",
		).
		WithTags("search", "rag", "knowledge", "indexing", "persistence").
		WithCapability("knowledge:ingest", "Add a document to the index").WithShortcut("i i").
		WithCapability("knowledge:ingest-buffer", "Ingest a buffer's content").WithShortcut("i b").
		WithCapability("knowledge:search", "Search the knowledge graph").WithShortcut("i s").
		WithCapability("knowledge:clear", "Wipe the current index").
		WithCapability("knowledge:status", "Get indexer status")

	plugin.Handle("knowledge:ingest", handleIngest)
	plugin.Handle("knowledge:ingest-buffer", handleIngestBuffer)
	plugin.Handle("knowledge:search", handleSearch)
	plugin.Handle("knowledge:clear", handleClear)
	plugin.Handle("knowledge:status", handleStatus)

	// Event Handlers
	plugin.Handle("buffer:update", handleBufferEvent)
	plugin.Handle("chat:message", handleChatEvent)
	plugin.Handle("task:create", handleTaskEvent)
	plugin.Handle("project:create", handleProjectEvent)

	// Backward compatibility handlers
	plugin.Handle("ingest", handleIngest)
	plugin.Handle("ingest-buffer", handleIngestBuffer)
	plugin.Handle("search", handleSearch)

	plugin.OnInit(func() error {
		plugin.Log("info", "Knowledge Graph Indexer initializing")

		// Subscribe to multiple activity streams
		topics := []string{"buffer:update", "chat:message", "task:create", "project:create"}
		for _, topic := range topics {
			subPayload, _ := json.Marshal(map[string]string{
				"topic": topic,
			})
			plugin.RouteMessage(AlloyMessage{
				Id:      "sub-" + strings.ReplaceAll(topic, ":", "-"),
				MsgType: "request",
				Method:  "subscribe",
				Sender:  "index",
				Target:  Some("events"),
				Payload: subPayload,
			})
		}

		// Register a dashboard widget
		plugin.RegisterWidget(AlloyWidget{
			Id:                "indexer-status",
			Title:             "Knowledge Graph",
			ContentType:       "text",
			Content:           []byte("Knowledge Graph Active"),
			RefreshIntervalMs: 15000,
		})

		updateStatus()
		return nil
	})

	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

func saveDoc(doc Document) {
	if doc.ID == "" {
		doc.ID = fmt.Sprintf("doc-%d", time.Now().UnixNano())
	}
	if doc.Timestamp == 0 {
		doc.Timestamp = time.Now().Unix()
	}
	if len(doc.Tags) == 0 {
		doc.Tags = generateTags(doc.Content)
	}

	data, _ := json.Marshal(doc)
	plugin.KVSet(DocPrefix+doc.ID, data)
	updateStatus()
}

func handleIngest(msg AlloyMessage) AlloyMessage {
	var doc Document
	if err := json.Unmarshal(msg.Payload, &doc); err != nil {
		return plugin.ErrorReply(msg, "invalid_document")
	}

	doc.Source = "manual"
	saveDoc(doc)

	plugin.Log("info", fmt.Sprintf("Indexed manual document: %s (%d bytes)", doc.Path, len(doc.Content)))

	return plugin.Reply(msg, map[string]string{
		"id":     doc.ID,
		"status": "indexed",
	})
}

func handleIngestBuffer(msg AlloyMessage) AlloyMessage {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request")
	}

	// Direct WIT call to buffer manager
	buf, ok := plugin.ReadBuffer(req.ID)
	if !ok {
		return plugin.ErrorReply(msg, "buffer_not_found")
	}

	doc := Document{
		ID:        "buf-" + buf.Id,
		Path:      "buffer://" + buf.Name,
		Content:   string(buf.Content),
		Source:    "buffer",
		Tags:      []string{"buffer"},
	}
	saveDoc(doc)

	plugin.Log("info", fmt.Sprintf("Indexed buffer: %s (%d bytes)", buf.Id, len(buf.Content)))

	return plugin.Reply(msg, map[string]string{
		"id":     doc.ID,
		"status": "indexed",
	})
}

func handleBufferEvent(msg AlloyMessage) AlloyMessage {
	var evt struct {
		BufferID string `json:"buffer_id"`
		Event    string `json:"event"`
	}
	if err := json.Unmarshal(msg.Payload, &evt); err == nil {
		if evt.Event == "update" || evt.Event == "append" || evt.Event == "create" {
			ingestPayload, _ := json.Marshal(map[string]string{"id": evt.BufferID})
			handleIngestBuffer(AlloyMessage{Payload: ingestPayload})
		}
	}
	return AlloyMessage{}
}

func handleChatEvent(msg AlloyMessage) AlloyMessage {
	var evt struct {
		Sender  string `json:"sender"`
		Content string `json:"content"`
		Context string `json:"context,omitempty"`
	}
	if err := json.Unmarshal(msg.Payload, &evt); err == nil {
		doc := Document{
			ID:      fmt.Sprintf("chat-%s-%d", evt.Sender, time.Now().UnixNano()),
			Path:    "chat://" + evt.Sender,
			Content: evt.Content,
			Source:  "chat",
			Author:  evt.Sender,
			Tags:    []string{"chat", "message"},
		}
		saveDoc(doc)
		plugin.Log("debug", "Auto-indexed chat message from "+evt.Sender)
	}
	return AlloyMessage{}
}

func handleTaskEvent(msg AlloyMessage) AlloyMessage {
	var evt struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(msg.Payload, &evt); err == nil {
		doc := Document{
			ID:      "task-" + evt.ID,
			Path:    "task://" + evt.ID,
			Content: evt.Title + "\n" + evt.Description,
			Source:  "task",
			Tags:    []string{"task", "todo"},
		}
		saveDoc(doc)
		plugin.Log("debug", "Auto-indexed task: "+evt.Title)
	}
	return AlloyMessage{}
}

func handleProjectEvent(msg AlloyMessage) AlloyMessage {
	var evt struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(msg.Payload, &evt); err == nil {
		doc := Document{
			ID:      "proj-" + evt.ID,
			Path:    "project://" + evt.ID,
			Content: "Project created: " + evt.Name,
			Source:  "project",
			Tags:    []string{"project", "meta"},
		}
		saveDoc(doc)
		plugin.Log("debug", "Auto-indexed project: "+evt.Name)
	}
	return AlloyMessage{}
}

func handleSearch(msg AlloyMessage) AlloyMessage {
	var req SearchRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_search_request")
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	queryTerms := strings.Fields(strings.ToLower(req.Query))
	results := []SearchResult{}

	keys := plugin.KVList(DocPrefix)
	for _, key := range keys {
		data, ok := plugin.KVGet(key)
		if !ok {
			continue
		}

		var doc Document
		if err := json.Unmarshal(data, &doc); err != nil {
			continue
		}

		score := 0.0
		contentLower := strings.ToLower(doc.Content)
		
		for _, term := range queryTerms {
			// Content match
			if strings.Contains(contentLower, term) {
				score += 2.0
			}
			// Tag match (higher weight)
			for _, tag := range doc.Tags {
				if strings.Contains(strings.ToLower(tag), term) {
					score += 3.0
				}
			}
			// Source match
			if strings.Contains(strings.ToLower(doc.Source), term) {
				score += 1.0
			}
		}

		if score > 0 {
			results = append(results, SearchResult{
				Document: doc,
				Score:    score,
			})
		}
	}

	// Simple sort by score (since we can't easily sort in guest without custom sort boilerplate)
	// For now, just return what we found up to limit
	if len(results) > req.Limit {
		results = results[:req.Limit]
	}

	return plugin.Reply(msg, results)
}

func handleClear(msg AlloyMessage) AlloyMessage {
	keys := plugin.KVList(DocPrefix)
	for _, key := range keys {
		plugin.KVDelete(key)
	}
	updateStatus()
	return plugin.Reply(msg, map[string]string{"status": "cleared"})
}

func handleStatus(msg AlloyMessage) AlloyMessage {
	keys := plugin.KVList(DocPrefix)
	return plugin.Reply(msg, map[string]interface{}{
		"document_count": len(keys),
		"status":         "active",
		"persistence":    "kv-enabled",
	})
}

func generateTags(content string) []string {
	commonKeywords := []string{"func", "struct", "package", "error", "interface", "map", "chan", "meeting", "deployment", "bug", "feature"}
	tags := []string{}
	contentLower := strings.ToLower(content)

	for _, kw := range commonKeywords {
		if strings.Contains(contentLower, kw) {
			tags = append(tags, kw)
		}
	}

	return tags
}

func updateStatus() {
	keys := plugin.KVList(DocPrefix)
	status := fmt.Sprintf("Graph Size: %d artifacts\nLast Activity: %s", 
		len(keys), time.Now().Format("15:04:05"))
	plugin.UpdateWidget("indexer-status", []byte(status))
}
