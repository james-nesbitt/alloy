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
	Type        string            `json:"type"` // "command", "document", "buffer", "chat", "presence", "semantic"
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

type LibrarianSearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
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
	Cursors   map[string]interface{} `json:"cursors,omitempty"`
}

type BufferListResponse struct {
	Buffers []Buffer `json:"buffers"`
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
	} else if req.Limit > 100 {
		req.Limit = 100
	}

	query := strings.ToLower(req.Query)
	results := []OmniResult{}
	
	// Budget intermediate allocations
	maxResultsBudget := req.Limit * 10
	if maxResultsBudget < 200 {
		maxResultsBudget = 200
	} else if maxResultsBudget > 2000 {
		maxResultsBudget = 2000
	}
	maxCapsBudget := maxResultsBudget * 4

	// 1. Get ALL system capabilities (Commands) filtered by context
	contextID := msg.ContextID()
	caps := plugin.GetAllCapabilities(msg.Actor, contextID)
	
	if len(caps) > maxCapsBudget {
		caps = caps[:maxCapsBudget]
	}

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
			if len(results) >= maxResultsBudget {
				break
			}
		}
	}

	// 2. Get Open Buffers
	bufListPayload, _ := json.Marshal(map[string]any{
		"include_content": false,
		"limit":           400,
	})
	bufResp := plugin.Call(AlloyMessage{
		Id:      fmt.Sprintf("omni-buf-%d", time.Now().UnixNano()),
		MsgType: "request",
		Method:  "buffer:list",
		Sender:  "omni-palette",
		Target:  Some("buffer"),
		Payload: bufListPayload,
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
					score += 50.0 
				}

				if score > 0 {
					collaborators := ""
					if len(buf.Cursors) > 0 {
						users := make([]string, 0, len(buf.Cursors))
						for u := range buf.Cursors {
							users = append(users, u)
						}
						collaborators = " | Collaborators: " + strings.Join(users, ", ")
					}

					results = append(results, OmniResult{
						ID:          buf.ID,
						Title:       buf.Name,
						Description: fmt.Sprintf("Open Buffer (%s)%s", buf.MimeType, collaborators),
						Type:        "buffer",
						Score:       score,
						Metadata: map[string]string{
							"action":    "switch",
							"buffer_id": buf.ID,
						},
					})

					if len(results) >= maxResultsBudget {
						break
					}
				}
			}
		}
	}

	// 3. Get Workspaces/Projects
	wsResp := plugin.Call(AlloyMessage{
		Id:      fmt.Sprintf("omni-ws-%d", time.Now().UnixNano()),
		MsgType: "request",
		Method:  "project:list-workspaces",
		Sender:  "omni-palette",
		Target:  Some("project"),
		Payload: []byte("{}"),
	})

	if wsResp.Method != "error" && len(wsResp.Payload) > 0 {
		var wsList struct {
			Workspaces []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"workspaces"`
		}
		if err := json.Unmarshal(wsResp.Payload, &wsList); err == nil {
			for _, ws := range wsList.Workspaces {
				score := 0.0
				name := strings.ToLower(ws.Name)

				if query == "" {
					score = 0.3
				} else if strings.Contains(name, query) {
					score += 15.0 
					if strings.HasPrefix(name, query) {
						score += 5.0
					}
				}

				if score > 0 {
					results = append(results, OmniResult{
						ID:          ws.ID,
						Title:       ws.Name,
						Description: "Switch to project context",
						Type:        "project",
						Score:       score,
						Metadata: map[string]string{
							"action": "switch-context",
							"id":     ws.ID,
						},
					})
					if len(results) >= maxResultsBudget {
						break
					}
				}
			}
		}
	}

	// 4. Get Team Presence
	presenceResp := plugin.Call(AlloyMessage{
		Id:      fmt.Sprintf("omni-pres-%d", time.Now().UnixNano()),
		MsgType: "request",
		Method:  "team-presence:list",
		Sender:  "omni-palette",
		Target:  Some("team-presence"),
		Payload: []byte("{}"),
	})

	if presenceResp.Method != "error" && len(presenceResp.Payload) > 0 {
		var presList []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Status   string `json:"status"`
			Activity string `json:"activity"`
		}
		if err := json.Unmarshal(presenceResp.Payload, &presList); err == nil {
			for _, p := range presList {
				if p.ID == msg.Actor {
					continue
				}

				score := 0.0
				name := strings.ToLower(p.Name)
				activity := strings.ToLower(p.Activity)

				if query == "" {
					score = 0.2
				} else if strings.Contains(name, query) || strings.Contains(activity, query) {
					score += 12.0
				}

				if score > 0 {
					results = append(results, OmniResult{
						ID:          "presence:" + p.ID,
						Title:       p.Name + " (" + p.Status + ")",
						Description: "Activity: " + p.Activity,
						Type:        "presence",
						Score:       score,
						Metadata: map[string]string{
							"action":  "dm",
							"user_id": p.ID,
						},
					})
				}
			}
		}
	}

	// 5. Query Librarian (Semantic Search)
	if query != "" {
		libSearchReq := LibrarianSearchRequest{Query: req.Query, Limit: 5}
		libPayload, _ := json.Marshal(libSearchReq)
		libResp := plugin.Call(AlloyMessage{
			Id:      "omni-lib-" + fmt.Sprint(time.Now().UnixNano()),
			MsgType: "request",
			Method:  "librarian:search",
			Sender:  "omni-palette",
			Target:  Some("librarian"),
			Payload: libPayload,
		})

		if libResp.Method != "error" && len(libResp.Payload) > 0 {
			var libResults []CompatSearchResult
			if err := json.Unmarshal(libResp.Payload, &libResults); err == nil {
				plugin.Log("info", fmt.Sprintf("Omni-Palette: Librarian returned %d results", len(libResults)))
				for _, res := range libResults {
					results = append(results, OmniResult{
						ID:          res.Document.ID,
						Title:       "Sem: " + res.Document.Path,
						Description: res.Document.Content,
						Type:        "semantic",
						Score:       res.Score * 50.0, 
						Metadata: map[string]string{
							"action": "open",
							"id":     res.Document.ID,
						},
					})
				}
			}
		}
	}

	// 6. Query Knowledge Graph (Indexer) if available
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
		var idxResults []CompatSearchResult
		if err := json.Unmarshal(idxResp.Payload, &idxResults); err == nil {
			for _, res := range idxResults {
				results = append(results, OmniResult{
					ID:          res.Document.ID,
					Title:       res.Document.Path,
					Description: summarize(res.Document.Content),
					Type:        "document",
					Score:       res.Score * 0.8,
					Metadata: map[string]string{
						"action": "open",
						"path":   res.Document.Path,
					},
				})
			}
		}
	}

	// Sort results by score (descending)
	plugin.Log("info", fmt.Sprintf("Omni-Palette: Sorting %d results", len(results)))
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

	finalResp, _ := json.Marshal(results)
	plugin.Log("info", "Omni-Palette Final Results: "+string(finalResp))

	return plugin.Reply(msg, results)
}

func summarize(content string) string {
	if len(content) > 100 {
		return content[:97] + "..."
	}
	return content
}
