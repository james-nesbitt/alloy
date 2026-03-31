//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// OmniResult represents a unified search result.
type OmniResult struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Type        string            `json:"type"` // "command", "document", "buffer", "chat"
	Score       float64           `json:"score"`
	Shortcut    string            `json:"shortcut,omitempty"`
	Source      string            `json:"source,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type OmniSearchRequest struct {
	Query    string `json:"query"`
	Limit    int    `json:"limit,omitempty"`
	BufferID string `json:"buffer_id,omitempty"` // Context: currently active buffer
}

// IndexSearchRequest from the index plugin
type IndexSearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

// Buffer from the buffer plugin
type Buffer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	MimeType  string `json:"mime_type"`
	Timestamp int64  `json:"last_modified"`
}

type BufferListResponse struct {
	Buffers []Buffer `json:"buffers"`
}

// SearchResult from the index plugin
type IndexDocument struct {
	ID        string   `json:"id"`
	Path      string   `json:"path"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	Timestamp int64    `json:"timestamp"`
	Author    string   `json:"author,omitempty"`
	Source    string   `json:"source,omitempty"`
}

type IndexSearchResult struct {
	Document IndexDocument `json:"document"`
	Score    float64       `json:"score"`
}

var (
	plugin *Plugin
)

func main() {
	plugin = NewPlugin("omni-palette").
		WithMetadata(
			"Omni-Palette",
			"Unified universal search and command surface",
			"0.1.0",
			"Alloy Team",
		).
		WithTags("search", "navigation", "palette", "commands").
		WithCapability("omni:search", "Search across commands, documents, and system state").WithShortcut("Ctrl+P")

	plugin.Handle("omni:search", handleOmniSearch)
	plugin.Handle("search", handleOmniSearch) // Alias

	plugin.OnInit(func() error {
		plugin.Log("info", "Omni-Palette initializing")
		
		// Register a mini dashboard widget to show search stats
		plugin.RegisterWidget(AlloyWidget{
			Id:          "omni-stats",
			Title:       "Omni-Palette",
			ContentType: "text",
			Content:     []byte("Omni Active (Ctrl+P)"),
		})
		
		return nil
	})

	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

func handleOmniSearch(msg AlloyMessage) AlloyMessage {
	var req OmniSearchRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_search_request")
	}

	if req.Limit <= 0 {
		req.Limit = 15
	}
	
	query := strings.ToLower(req.Query)
	results := []OmniResult{}

	// 1. Get ALL system capabilities (Commands) filtered by context
	contextID := msg.ContextID()
	caps := plugin.GetAllCapabilities(msg.Actor, contextID)
	for _, cap := range caps {
		score := 0.0
		method := strings.ToLower(cap.Method)
		desc := strings.ToLower(cap.Description)

		if query == "" {
			score = 1.0 // Base score for empty search (list all)
		} else {
			if strings.Contains(method, query) {
				score += 10.0
				if strings.HasPrefix(method, query) {
					score += 5.0
				}
			}
			if strings.Contains(desc, query) {
				score += 5.0
			}
		}

		if score > 0 {
			shortcut := ""
			if cap.Shortcut.IsSome() {
				shortcut = cap.Shortcut.Unwrap()
			}

			results = append(results, OmniResult{
				ID:          cap.Method,
				Title:       cap.Method,
				Description: cap.Description,
				Type:        "command",
				Score:       score,
				Shortcut:    shortcut,
				Metadata: map[string]string{
					"action": "execute",
				},
			})
		}
	}

	// 2. Get Open Buffers
	bufResp := plugin.Call(AlloyMessage{
		Id:      fmt.Sprintf("omni-buf-%d", time.Now().UnixNano()),
		MsgType: "request",
		Method:  "buffer:list",
		Sender:  "omni-palette",
		Target:  Some("buffer"),
		Payload: []byte("{}"),
	})

	if bufResp.Method != "error" && len(bufResp.Payload) > 0 {
		var bufList BufferListResponse
		if err := json.Unmarshal(bufResp.Payload, &bufList); err == nil {
			for _, buf := range bufList.Buffers {
				score := 0.0
				name := strings.ToLower(buf.Name)
				
				if query == "" {
					score = 0.5
				} else if strings.Contains(name, query) {
					score += 8.0
					if strings.HasPrefix(name, query) {
						score += 4.0
					}
				}

				// Context-Aware Boost: Prioritize currently active buffer
				if req.BufferID != "" && buf.ID == req.BufferID {
					score += 50.0 // Massive boost for local context
				}

				if score > 0 {
					results = append(results, OmniResult{
						ID:          buf.ID,
						Title:       buf.Name,
						Description: fmt.Sprintf("Open Buffer (%s)", buf.MimeType),
						Type:        "buffer",
						Score:       score,
						Shortcut:    "", // Buffers don't have default shortcuts yet
						Metadata: map[string]string{
							"action":    "switch",
							"buffer_id": buf.ID,
						},
					})
				}
			}
		}
	}

	// 3. Query Knowledge Graph (Indexer) if available
	idxSearchReq := IndexSearchRequest{Query: req.Query, Limit: req.Limit}
	payload, _ := json.Marshal(idxSearchReq)
	
	idxResp := plugin.Call(AlloyMessage{
		Id:      fmt.Sprintf("omni-idx-%d", time.Now().UnixNano()),
		MsgType: "request",
		Method:  "knowledge:search",
		Sender:  "omni-palette",
		Target:  Some("index"),
		Payload: payload,
	})

	if idxResp.Method != "error" && len(idxResp.Payload) > 0 {
		var idxResults []IndexSearchResult
		if err := json.Unmarshal(idxResp.Payload, &idxResults); err == nil {
			for _, res := range idxResults {
				results = append(results, OmniResult{
					ID:          res.Document.ID,
					Title:       res.Document.Path,
					Description: summarize(res.Document.Content),
					Type:        "document",
					Score:       res.Score * 0.8, // Slightly weight documents lower than commands
					Source:      res.Document.Source,
					Metadata: map[string]string{
						"action": "open",
						"path":   res.Document.Path,
						"author": res.Document.Author,
						"tags":   strings.Join(res.Document.Tags, ","),
					},
				})
			}
		}
	}

	// 4. Sort results by score (descending)
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

func summarize(content string) string {
	if len(content) > 100 {
		return content[:97] + "..."
	}
	return content
}
