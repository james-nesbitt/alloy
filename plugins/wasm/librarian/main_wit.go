//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// LibrarianSearchRequest represents a semantic search query.
type LibrarianSearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

// CompatSearchResult matches the format for both Librarian and Indexer
type CompatSearchResult struct {
	Document struct {
		ID      string   `json:"id"`
		Path    string   `json:"path"`
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
	} `json:"document"`
	Score float64 `json:"score"`
}

type IndexItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	Embedding []float64 `json:"embedding,omitempty"`
	Timestamp int64     `json:"timestamp"`
}

const (
	IndexPrefix = "lib:idx:"
)

var (
	plugin *Plugin
)

func main() {
	plugin = NewPlugin("librarian").
		SetBackground(true).
		WithMetadata(
			"Librarian",
			"Semantic indexing and retrieval service",
			"0.1.0",
			"Alloy Team",
		).
		WithTags("search", "indexing", "vector", "memory", "semantic").
		WithCapability("librarian:search", "Semantic search across workspace events and content").WithShortcut("Ctrl+Shift+F").
		WithCapability("librarian:index-buffer", "Manually index a buffer").
		WithCapability("librarian:status", "Get librarian health and stats").
		WithCapability("knowledge:search", "Legacy semantic/keyword search interface")

	plugin.Handle("librarian:search", handleSearch)
	plugin.Handle("knowledge:search", handleSearch)
	plugin.Handle("librarian:index-buffer", handleIndexBuffer)
	plugin.Handle("librarian:status", handleStatus)
	
	// Event Handlers
	plugin.Handle("buffer:update", handleBufferUpdateEvent)
	plugin.Handle("chat:message", handleChatMessageEvent)

	plugin.OnInit(func() error {
		plugin.Log("info", "Librarian initializing")
		
		// Register a dashboard widget
		plugin.RegisterWidget(AlloyWidget{
			Id:          "librarian-stats",
			Title:       "Librarian Memory",
			ContentType: "text",
			Content:     []byte("Librarian Active | Semantic Indexing online"),
		})
		
		return nil
	})

	plugin.OnStart(func() {
		// Subscribe to interesting topics
		topics := []string{"buffer:update", "chat:message"}
		for _, topic := range topics {
			subPayload, _ := json.Marshal(map[string]string{"topic": topic})
			plugin.RouteMessage(AlloyMessage{
				MsgType: "request",
				Method:  "events:subscribe",
				Sender:  "librarian",
				Target:  Some("events:subscribe"),
				Payload: subPayload,
			})
		}
	})

	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

func handleSearch(msg AlloyMessage) AlloyMessage {
	plugin.Log("info", "LibrarianSearch: received request")
	var req LibrarianSearchRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_search_request")
	}

	if req.Limit <= 0 {
		req.Limit = 5
	}

	// 1. Embed the query
	embedReq, _ := json.Marshal(map[string]string{"text": req.Query})
	embedResp := plugin.Call(AlloyMessage{
		Id:      "lib-search-embed-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "request",
		Method:  "ai:embed",
		Sender:  "librarian",
		Target:  Some("ai"),
		Payload: embedReq,
	})

	if embedResp.Method == "error" {
		plugin.Log("error", "LibrarianSearch: embedding failed")
		return plugin.ErrorReply(msg, "embedding_failed")
	}

	var embedData struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.Unmarshal(embedResp.Payload, &embedData); err != nil {
		return plugin.ErrorReply(msg, "failed_to_parse_embedding")
	}

	// 2. Fetch all indexed items and calculate similarity
	results := []CompatSearchResult{}
	keys := plugin.KVList(IndexPrefix)
	plugin.Log("info", fmt.Sprintf("Librarian Search: found %d indexed items", len(keys)))
	
	for _, key := range keys {
		data, ok := plugin.KVGet(key)
		if !ok { continue }

		var item IndexItem
		if err := json.Unmarshal(data, &item); err != nil { continue }
		if len(item.Embedding) == 0 { continue }

		score := cosineSimilarity(embedData.Embedding, item.Embedding)
		plugin.Log("info", fmt.Sprintf("Librarian Similarity with %s: %.4v", item.Title, score))
		
		if score > 0.6 { // Minimum similarity threshold
			res := CompatSearchResult{
				Score: score,
			}
			res.Document.ID = item.ID
			res.Document.Path = item.Title
			res.Document.Content = item.Content
			res.Document.Tags = []string{item.Type}
			results = append(results, res)
		}
	}

	// 3. Sort results
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Score < results[j].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > req.Limit {
		results = results[:req.Limit]
	}

	return plugin.Reply(msg, results)
}

func handleIndexBuffer(msg AlloyMessage) AlloyMessage {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request")
	}

	buf, ok := plugin.ReadBuffer(req.ID)
	if !ok {
		return plugin.ErrorReply(msg, "buffer_not_found")
	}

	err := indexContent("buffer:"+buf.Id, buf.Name, "buffer", string(buf.Content))
	if err != nil {
		return plugin.ErrorReply(msg, "indexing_failed: "+err.Error())
	}

	return plugin.Reply(msg, map[string]string{"status": "indexed"})
}

func handleStatus(msg AlloyMessage) AlloyMessage {
	keys := plugin.KVList(IndexPrefix)
	return plugin.Reply(msg, map[string]interface{}{
		"indexed_documents": len(keys),
		"status": "online",
	})
}

func handleBufferUpdateEvent(msg AlloyMessage) AlloyMessage {
	var evt struct {
		BufferID string `json:"buffer_id"`
		Event    string `json:"event"`
	}
	if err := json.Unmarshal(msg.Payload, &evt); err == nil {
		if evt.Event == "update" || evt.Event == "create" || evt.Event == "save" {
			buf, ok := plugin.ReadBuffer(evt.BufferID)
			if ok {
				indexContent("buffer:"+buf.Id, buf.Name, "buffer", string(buf.Content))
			}
		}
	}
	return AlloyMessage{}
}

func handleChatMessageEvent(msg AlloyMessage) AlloyMessage {
	var evt struct {
		Sender  string `json:"sender"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(msg.Payload, &evt); err == nil {
		id := fmt.Sprintf("chat-%s-%d", evt.Sender, time.Now().UnixNano())
		indexContent(id, "Chat from "+evt.Sender, "chat", evt.Content)
	}
	return AlloyMessage{}
}

func indexContent(id, title, docType, content string) error {
	if len(strings.TrimSpace(content)) < 5 {
		return nil 
	}

	// 1. Get embedding
	embedReq, _ := json.Marshal(map[string]string{"text": content})
	embedResp := plugin.Call(AlloyMessage{
		Id:      "lib-embed-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "request",
		Method:  "ai:embed",
		Sender:  "librarian",
		Target:  Some("ai"),
		Payload: embedReq,
	})

	if embedResp.Method == "error" {
		return fmt.Errorf("ai_embedding_failed")
	}

	var embedData struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.Unmarshal(embedResp.Payload, &embedData); err != nil {
		return err
	}

	// 2. Store
	item := IndexItem{
		ID:        id,
		Title:     title,
		Type:      docType,
		Content:   content,
		Embedding: embedData.Embedding,
		Timestamp: time.Now().Unix(),
	}

	data, _ := json.Marshal(item)
	plugin.KVSet(IndexPrefix+id, data)
	
	plugin.Log("info", fmt.Sprintf("Librarian: Indexed %s (%s)", title, id))
	return nil
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func summarize(content string) string {
	if len(content) > 100 {
		return content[:97] + "..."
	}
	return content
}
