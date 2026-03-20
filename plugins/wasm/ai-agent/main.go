package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

type ChatMessage struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type ProviderConfig struct {
	Name    string `json:"name"`
	APIKey  string `json:"api_key,omitempty"`
	URL     string `json:"url"`
	Model   string `json:"model"`
	Type    string `json:"type"` // "openai", "anthropic", "ollama"
}

func init() {
	wasm.SetHandler(handleMessage)
	wasm.SetCapabilities([]wasm.Capability{
		{Method: "config:set", Description: "Configure AI provider", Shortcut: "a c s", Annotations: map[string]string{"group": "ai"}},
		{Method: "config:get", Description: "Get current configuration", Shortcut: "a c g", Annotations: map[string]string{"group": "ai"}},
		{Method: "query", Description: "Query the AI directly", Shortcut: "a q", Annotations: map[string]string{"group": "ai"}},
		{Method: "summarize", Description: "Summarize provided text Content", Shortcut: "a s", Annotations: map[string]string{"group": "ai"}},
		{Method: "chat:message", Description: "Reactive chat handler", Annotations: map[string]string{"group": "ai"}},
	})
}

func main() {
	// Subscribe to chat events
	subReq, _ := json.Marshal(map[string]string{"topic": "chat:message"})
	wasm.RouteMessage(wasm.Message{
		ID:      "sub-ai-chat",
		Type:    "request",
		Sender:  "plugin-ai-agent",
		Target:  "plugin-events",
		Method:  "subscribe",
		Payload: subReq,
	})

	wasm.SleepForever()
}

func handleMessage(msg wasm.Message) wasm.Message {
	switch msg.Method {
	case "config:set":
		var cfg ProviderConfig
		if err := json.Unmarshal(msg.Payload, &cfg); err != nil {
			return errorResponse(msg, "invalid config")
		}
		wasm.KVSet("config", msg.Payload)
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-ai-agent", Target: msg.Sender,
			Payload: []byte(`{"status":"ok"}`),
		}

	case "config:get":
		cfg := wasm.KVGet("config")
		if cfg == nil {
			return errorResponse(msg, "not configured")
		}
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-ai-agent", Target: msg.Sender,
			Payload: cfg,
		}

	case "query":
		var req struct {
			Prompt string `json:"prompt"`
		}
		json.Unmarshal(msg.Payload, &req)
		
		response, err := performLLMQuery(req.Prompt)
		if err != nil {
			return errorResponse(msg, err.Error())
		}
		
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-ai-agent", Target: msg.Sender,
			Payload: []byte(`{"response":"` + response + `"}`),
		}

	case "chat:message":
		var chatMsg ChatMessage
		if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
			return wasm.Message{}
		}

		if chatMsg.Sender == "plugin-ai-agent" || chatMsg.Sender == "plugin-chat" {
			return wasm.Message{}
		}

		if strings.HasPrefix(strings.ToLower(chatMsg.Content), "ai:") {
			prompt := strings.TrimSpace(chatMsg.Content[3:])
			response, err := performLLMQuery(prompt)
			if err != nil {
				sendChatMessage(chatMsg.Channel, "AI Error: "+err.Error())
			} else {
				sendChatMessage(chatMsg.Channel, response)
			}
		}
		return wasm.Message{}

	case "summarize":
		// Multi-step orchestration: get history -> summarize -> respond
		var req struct {
			Channel string `json:"channel"`
		}
		json.Unmarshal(msg.Payload, &req)
		
		// In a real scenario, this would be more complex (async requests)
		// For now, return a placeholder
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-ai-agent", Target: msg.Sender,
			Payload: []byte(`{"summary":"Summarization orchestration is being developed."}`),
		}

	default:
		return wasm.Message{}
	}
}

func performLLMQuery(prompt string) (string, error) {
	cfgData := wasm.KVGet("config")
	var cfg ProviderConfig
	if cfgData == nil {
		// Default mock for testing if not configured
		cfg = ProviderConfig{Type: "mock", Model: "test-model"}
	} else {
		json.Unmarshal(cfgData, &cfg)
	}

	// Generic LLM fetch logic
	switch cfg.Type {
	case "mock":
		return "Mock AI response to: " + prompt, nil
	case "ollama":
		reqBody, _ := json.Marshal(map[string]any{
			"model":  cfg.Model,
			"prompt": prompt,
			"stream": false,
		})
		resp, err := wasm.Fetch(wasm.FetchRequest{
			Method: "POST",
			URL:    cfg.URL + "/api/generate",
			Body:   reqBody,
		})
		if err != nil { return "", err }
		var res struct { Response string `json:"response"` }
		json.Unmarshal(resp.Body, &res)
		return res.Response, nil

	case "openai":
		// simplified OpenAI format
		reqBody, _ := json.Marshal(map[string]any{
			"model": cfg.Model,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
		})
		_, err := wasm.Fetch(wasm.FetchRequest{
			Method: "POST",
			URL:    "https://api.openai.com/v1/chat/completions",
			Headers: map[string]string{
				"Authorization": "Bearer " + cfg.APIKey,
				"Content-Type":  "application/json",
			},
			Body: reqBody,
		})
		if err != nil { return "", err }
		// Parse OpenAI response... (omitted for brevity)
		return "OpenAI response processing placeholder", nil

	default:
		return "", fmt.Errorf("unsupported provider type: %s", cfg.Type)
	}
}

func sendChatMessage(channel, content string) {
	chatReq, _ := json.Marshal(map[string]string{
		"channel": channel,
		"content": content,
	})
	wasm.RouteMessage(wasm.Message{
		ID:        fmt.Sprintf("ai-resp-%d", time.Now().UnixNano()),
		Type:      "request",
		Sender:    "plugin-ai-agent",
		Target:    "plugin-chat",
		Method:    "send",
		Payload:   chatReq,
		Timestamp: time.Now().Unix(),
	})
}

func errorResponse(msg wasm.Message, err string) wasm.Message {
	return wasm.Message{
		ID:     msg.ID + "-resp",
		Type:   "response",
		Sender: "plugin-ai-agent",
		Target: msg.Sender,
		Payload: []byte(fmt.Sprintf(`{"error":"%s"}`, err)),
	}
}
