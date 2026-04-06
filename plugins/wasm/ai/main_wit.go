//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"strings"
)

// AIProviderType represents the type of AI provider.
type AIProviderType string

const (
	ProviderOllama    AIProviderType = "ollama"
	ProviderOpenAI    AIProviderType = "openai"
	ProviderAnthropic AIProviderType = "anthropic"
	ProviderMock      AIProviderType = "mock"
)

// ProviderConfig represents the configuration for an AI provider.
type ProviderConfig struct {
	Name    string         `json:"name"`
	APIKey  string         `json:"api_key,omitempty"`
	URL     string         `json:"url"`
	Model   string         `json:"model"`
	Type    AIProviderType `json:"type"` 
}

var (
	plugin      *Plugin
	configStore = NewKVStore[ProviderConfig]("ai:config")
)

// KVStore provides type-safe KV storage.
type KVStore[T any] struct {
	prefix string
}

// NewKVStore creates a new KVStore instance.
func NewKVStore[T any](prefix string) *KVStore[T] {
	return &KVStore[T]{prefix: prefix}
}

// Get retrieves a value from KV storage.
func (s *KVStore[T]) Get(key string) (T, error) {
	var result T
	data, ok := plugin.KVGet(s.prefix + ":" + key)
	if !ok || data == nil {
		return result, fmt.Errorf("not found")
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	return result, nil
}

// Set stores a value in KV storage.
func (s *KVStore[T]) Set(key string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !plugin.KVSet(s.prefix+":"+key, data) {
		return fmt.Errorf("failed to set value")
	}
	return nil
}

func main() {
	plugin = NewPlugin("ai")
	plugin.Log(LogLevelInfo, "AI main starting")
	
	plugin.WithMetadata(
			"AI Agent",
			"Provides AI capabilities",
			"0.1.0",
			"Alloy Team",
		).
		WithCapability("ai:query", "Query the AI").
		WithShortcut("a q").
		WithCapability("ai:embed", "Generate vector embeddings for text").
		WithCapability("ai:config:get", "Get current configuration").
		WithCapability("ai:config:set", "Configure AI provider").
		WithCapability("chat:message", "Handle incoming chat messages").
		WithCapability("summarize-buffer", "Summarize the content of a buffer")

	// Subscription to chat messages
	plugin.OnInit(func() error {
		plugin.Log(LogLevelInfo, "AI plugin subscribing to chat:message")
		subReq, _ := json.Marshal(map[string]string{"topic": "chat:message"})
		plugin.RouteMessage(AlloyMessage{
			MsgType: "request",
			Method:  "subscribe",
			Sender:  "ai",
			Target:  Some("events"),
			Payload: subReq,
		})

		// Register AI Status widget for dashboard tests
		plugin.Log(LogLevelInfo, "AI plugin registering status widget")
		plugin.RegisterWidget(AlloyWidget{
			Id:                "ai-status",
			Title:             "AI Assistant",
			ContentType:       "markdown",
			Content:           []byte("# AI Status\nActive and listening."),
			RefreshIntervalMs: 0,
		})

		// Initialize default config if none exists
		_, err := configStore.Get("current")
		if err != nil {
			_ = configStore.Set("current", ProviderConfig{
				Type:  ProviderMock,
				Model: "test-model",
			})
		}

		return nil
	})

	// Core handlers
	plugin.Handle("query", handleQuery)
	plugin.Handle("ai:query", handleQuery)
	plugin.Handle("embed", handleEmbed)
	plugin.Handle("ai:embed", handleEmbed)
	plugin.Handle("config:get", handleConfigGet)
	plugin.Handle("ai:config:get", handleConfigGet)
	plugin.Handle("config:set", handleConfigSet)
	plugin.Handle("ai:config:set", handleConfigSet)

	// Reaction logic
	plugin.Handle("chat:message", func(msg AlloyMessage) AlloyMessage {
		plugin.Log(LogLevelDebug, "AI received message topic: chat:message")
		
		var chatMsg struct {
			Sender  string `json:"sender"`
			Content string `json:"content"`
			Channel string `json:"channel"`
		}
		
		if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
			plugin.Log(LogLevelError, "AI failed to unmarshal chat message: "+err.Error())
			return plugin.Reply(msg, "error")
		}

		plugin.Log(LogLevelDebug, fmt.Sprintf("AI message from %s: %s", chatMsg.Sender, chatMsg.Content))

		if chatMsg.Sender != "ai" && strings.HasPrefix(strings.ToLower(chatMsg.Content), "ai:") {

			plugin.Log(LogLevelInfo, "AI responding to: "+chatMsg.Content)
			
			responseContent := "Mock AI response. Current project: active."
			
			responseMsg := map[string]interface{}{
				"channel": chatMsg.Channel,
				"content": responseContent,
			}
			
			payload, _ := json.Marshal(responseMsg)
			
			plugin.RouteMessage(AlloyMessage{
				MsgType: "request",
				Method:  "chat:send",
				Sender:  "ai",
				Target:  Some("chat"),
				Payload: payload,
			})
			
			plugin.Log(LogLevelDebug, "AI sent response to chat plugin")
		}
		
		return plugin.Reply(msg, "ok")
	})

	plugin.Handle("summarize-buffer", func(msg AlloyMessage) AlloyMessage {
		plugin.Log(LogLevelInfo, "AI summarizing buffer")
		
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return plugin.ErrorReply(msg, "invalid_request")
		}

		// Mock summarization
		response := "This is a summary of buffer " + req.ID
		
		return plugin.Reply(msg, map[string]string{
			"response": response,
		})
	})

	plugin.Serve()
}

func handleQuery(msg AlloyMessage) AlloyMessage {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request")
	}

	// Get knowledge graph context (RAG)
	knowledgeContext := getKnowledgeContext(req.Prompt)

	response := "Mock AI response. "
	if knowledgeContext != "" {
		response = "Relevant Knowledge Graph Context:\n" + knowledgeContext + "\n\nUser Question: " + req.Prompt
	} else {
		response += "No specific context found for: " + req.Prompt
	}

	return plugin.Reply(msg, map[string]string{
		"response": response,
	})
}

func handleEmbed(msg AlloyMessage) AlloyMessage {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_embed_payload")
	}

	// Deterministic mock embedding
	size := 1536 
	embedding := make([]float64, size)
	seed := int64(len(req.Text))
	if len(req.Text) > 0 {
		seed += int64(req.Text[0])
	}
	for i := 0; i < size; i++ {
		embedding[i] = float64(((seed + int64(i)) % 100)) / 100.0
	}

	return plugin.Reply(msg, map[string]interface{}{
		"embedding": embedding,
	})
}

func handleConfigGet(msg AlloyMessage) AlloyMessage {
	cfg, err := configStore.Get("current")
	if err != nil {
		cfg = ProviderConfig{Type: ProviderMock, Model: "test-model"}
	}
	return plugin.Reply(msg, cfg)
}

func handleConfigSet(msg AlloyMessage) AlloyMessage {
	var cfg ProviderConfig
	if err := json.Unmarshal(msg.Payload, &cfg); err != nil {
		return plugin.ErrorReply(msg, "invalid_config")
	}
	_ = configStore.Set("current", cfg)
	return plugin.Reply(msg, "ok")
}

func getKnowledgeContext(query string) string {
	searchReq, _ := json.Marshal(map[string]interface{}{
		"query": query,
		"limit": 3,
	})

	msg := AlloyMessage{
		Id:      "ai-search",
		MsgType: "request",
		Method:  "knowledge:search",
		Sender:  "ai",
		Target:  Some("index"),
		Payload: searchReq,
	}

	resp := plugin.Call(msg)
	if resp.MsgType == "error" {
		return ""
	}

	type SearchResult struct {
		Document struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"document"`
	}

	var results []SearchResult
	if err := json.Unmarshal(resp.Payload, &results); err != nil {
		return ""
	}

	if len(results) == 0 {
		return ""
	}

	var contexts []string
	for _, res := range results {
		contexts = append(contexts, fmt.Sprintf("File: %s\nSnippet: %s",
			res.Document.Path, res.Document.Content))
	}

	return strings.Join(contexts, "\n---\n")
}
