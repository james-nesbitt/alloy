//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Document represents a semantically indexed file or snippet.
type Document struct {
	ID        string   `json:"id"`
	Path      string   `json:"path"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	Timestamp int64    `json:"timestamp"`
	Author    string   `json:"author,omitempty"`
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
	plugin    *Plugin
	documents = make(map[string]Document)
)

func main() {
	plugin = NewPlugin("index").
		WithMetadata(
			"Knowledge Graph Indexer",
			"Background semantic indexing for collaborative work",
			"0.1.0",
			"Alloy Team",
		).
		WithTags("search", "rag", "knowledge", "indexing").
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

	// Backward compatibility handlers
	plugin.Handle("ingest", handleIngest)
	plugin.Handle("ingest-buffer", handleIngestBuffer)
	plugin.Handle("search", handleSearch)

	plugin.OnInit(func() error {
		plugin.Log("info", "Knowledge Graph Indexer initializing")

		// Register a dashboard widget
		plugin.RegisterWidget(AlloyWidget{
			Id:                "indexer-status",
			Title:             "Knowledge Graph",
			ContentType:       "text",
			Content:           []byte("No documents indexed"),
			RefreshIntervalMs: 15000,
		})

		return nil
	})

	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

func handleIngest(msg AlloyMessage) AlloyMessage {
	var doc Document
	if err := json.Unmarshal(msg.Payload, &doc); err != nil {
		return plugin.ErrorReply(msg, "invalid_document")
	}

	if doc.ID == "" {
		doc.ID = fmt.Sprintf("doc-%d", time.Now().UnixNano())
	}
	doc.Timestamp = time.Now().Unix()
	
	// Automatically generate basic tags from content if they don't exist
	if len(doc.Tags) == 0 {
		doc.Tags = generateTags(doc.Content)
	}

	documents[doc.ID] = doc
	updateStatus()

	plugin.Log("info", fmt.Sprintf("Indexed document: %s (%d bytes)", doc.Path, len(doc.Content)))

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
		Timestamp: time.Now().Unix(),
		Tags:      []string{"buffer"},
	}
	doc.Tags = append(doc.Tags, generateTags(doc.Content)...)

	documents[doc.ID] = doc
	updateStatus()

	plugin.Log("info", fmt.Sprintf("Indexed buffer: %s (%d bytes)", buf.Id, len(buf.Content)))

	return plugin.Reply(msg, map[string]string{
		"id":     doc.ID,
		"status": "indexed",
	})
}

func handleSearch(msg AlloyMessage) AlloyMessage {
	var req SearchRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_search_request")
	}

	if req.Limit <= 0 {
		req.Limit = 5
	}

	// Simple keyword/tag scoring since we don't have a vector engine in the guest yet
	queryTerms := strings.Fields(strings.ToLower(req.Query))
	results := []SearchResult{}

	for _, doc := range documents {
		score := 0.0
		contentLower := strings.ToLower(doc.Content)
		
		for _, term := range queryTerms {
			// Content match
			if strings.Contains(contentLower, term) {
				score += 1.0
			}
			// Tag match (higher weight)
			for _, tag := range doc.Tags {
				if strings.Contains(strings.ToLower(tag), term) {
					score += 2.0
				}
			}
		}

		if score > 0 {
			results = append(results, SearchResult{
				Document: doc,
				Score:    score,
			})
		}
	}

	// Limit results
	if len(results) > req.Limit {
		results = results[:req.Limit]
	}

	return plugin.Reply(msg, results)
}

func handleClear(msg AlloyMessage) AlloyMessage {
	documents = make(map[string]Document)
	updateStatus()
	return plugin.Reply(msg, map[string]string{"status": "cleared"})
}

func handleStatus(msg AlloyMessage) AlloyMessage {
	return plugin.Reply(msg, map[string]interface{}{
		"document_count": len(documents),
		"status":         "active",
	})
}

func generateTags(content string) []string {
	// Crude but functional tag generator for mock context
	commonKeywords := []string{"func", "struct", "package", "error", "interface", "map", "chan"}
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
	status := fmt.Sprintf("Index Size: %d documents\nLast Updated: %s", 
		len(documents), time.Now().Format("15:04:05"))
	plugin.UpdateWidget("indexer-status", []byte(status))
}
