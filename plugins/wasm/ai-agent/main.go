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
		{Method: "config:set", Description: "Configure AI provider (JSON)", Shortcut: "a c s", Annotations: map[string]string{"group": "ai"}},
		{Method: "config:get", Description: "Get current configuration", Shortcut: "a c g", Annotations: map[string]string{"group": "ai"}},
		{Method: "provider:set", Description: "Switch AI provider (ollama|openai|anthropic)", Shortcut: "a p s", Annotations: map[string]string{"group": "ai"}},
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
			return errorResponse(msg, "invalid config: "+err.Error())
		}
		// Validate config
		if cfg.Type == "" {
			cfg.Type = "ollama" // Default
		}
		if cfg.URL == "" && cfg.Type == "ollama" {
			cfg.URL = "http://127.0.0.1:11434"
		}
		if cfg.Model == "" {
			if cfg.Type == "ollama" {
				cfg.Model = "llama3"
			} else if cfg.Type == "openai" {
				cfg.Model = "gpt-4o"
			} else if cfg.Type == "anthropic" {
				cfg.Model = "claude-3-5-sonnet-20240620"
			}
		}

		updated, _ := json.Marshal(cfg)
		wasm.KVSet("config", updated)
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-ai-agent", Target: msg.Sender,
			Payload: []byte(`{"status":"ok","config":` + string(updated) + `}`),
		}

	case "config:get":
		cfgData := wasm.KVGet("config")
		if cfgData == nil {
			// Return default if not set
			defaultCfg, _ := json.Marshal(ProviderConfig{
				Type:  "ollama",
				URL:   "http://127.0.0.1:11434",
				Model: "llama3",
			})
			return wasm.Message{
				ID: msg.ID + "-resp", Type: "response", Sender: "plugin-ai-agent", Target: msg.Sender,
				Payload: defaultCfg,
			}
		}
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-ai-agent", Target: msg.Sender,
			Payload: cfgData,
		}

	case "provider:set":
		var req struct {
			Type  string `json:"type"`
			Model string `json:"model,omitempty"`
			URL   string `json:"url,omitempty"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return errorResponse(msg, "invalid payload")
		}
		
		cfgData := wasm.KVGet("config")
		var cfg ProviderConfig
		if cfgData != nil {
			json.Unmarshal(cfgData, &cfg)
		}
		
		cfg.Type = req.Type
		if req.Model != "" { cfg.Model = req.Model }
		if req.URL != "" { cfg.URL = req.URL }
		
		// Fill defaults if missing
		if cfg.URL == "" && cfg.Type == "ollama" { cfg.URL = "http://127.0.0.1:11434" }
		if cfg.Model == "" {
			switch cfg.Type {
			case "ollama": cfg.Model = "llama3"
			case "openai": cfg.Model = "gpt-4o"
			case "anthropic": cfg.Model = "claude-3-5-sonnet"
			}
		}
		
		updated, _ := json.Marshal(cfg)
		wasm.KVSet("config", updated)
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-ai-agent", Target: msg.Sender,
			Payload: []byte(`{"status":"ok","type":"` + cfg.Type + `","model":"` + cfg.Model + `"}`),
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
		// Orchestration: get project -> read buffers -> summarize
		projData := wasm.KVGet("shared:active-project")
		if projData == nil {
			return errorResponse(msg, "no active project to summarize")
		}

		var proj struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Buffers     []string `json:"buffers"`
		}
		json.Unmarshal(projData, &proj)

		if len(proj.Buffers) == 0 {
			response, err := performLLMQuery("Summarize this project based on its description: " + proj.Description)
			if err != nil { return errorResponse(msg, err.Error()) }
			return wasm.Message{
				ID: msg.ID + "-resp", Type: "response", Sender: "plugin-ai-agent", Target: msg.Sender,
				Payload: []byte(`{"summary":"` + response + `"}`),
			}
		}

		// Since we cannot easily do async-wait in this simple WASM handler yet,
		// we'll use a hack of sending a sequence of messages and using the KV to store intermediate results,
		// OR we can just fetch all content if we had a synchronous 'read' (which we don't have via host functions yet).
		
		// Wait, we DO have a way to route messages. 
		// For now, I will implement a simpler version that just summarizes the project metadata.
		// To truly summarize buffers, we need a 'shared' buffer access or synchronous read.
		
		summaryPrompt := fmt.Sprintf("Summarize the following project:\nName: %s\nDescription: %s\nFiles: %s", 
			proj.Name, proj.Description, strings.Join(proj.Buffers, ", "))
			
		response, err := performLLMQuery(summaryPrompt)
		if err != nil { return errorResponse(msg, err.Error()) }
		
		return wasm.Message{
			ID: msg.ID + "-resp", Type: "response", Sender: "plugin-ai-agent", Target: msg.Sender,
			Payload: []byte(`{"summary":"` + response + `"}`),
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

	// Fetch project context
	var projectContext string
	if projData := wasm.KVGet("shared:active-project"); projData != nil {
		var proj struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Buffers     []string `json:"buffers"`
			Channels    []string `json:"channels"`
		}
		if err := json.Unmarshal(projData, &proj); err == nil {
			projectContext = fmt.Sprintf("\nYou are currently working on project '%s' (%s).\n"+
				"This project includes these buffers: [%s] and these channels: [%s].\n",
				proj.Name, proj.Description, strings.Join(proj.Buffers, ", "), strings.Join(proj.Channels, ", "))
		}
	}

	fullPrompt := prompt
	if projectContext != "" {
		fullPrompt = projectContext + "\nUser Question: " + prompt
	}

	// Generic LLM fetch logic
	switch cfg.Type {
	case "mock":
		if projectContext != "" {
			return fmt.Sprintf("Mock AI response (with project context '%s') to: %s",
				projectContext[:20]+"...", prompt), nil
		}
		return "Mock AI response to: " + prompt, nil
	case "ollama":
		reqBody, _ := json.Marshal(map[string]any{
			"model": cfg.Model,
			"messages": []map[string]string{
				{"role": "user", "content": fullPrompt},
			},
			"stream": false,
		})
		resp, err := wasm.Fetch(wasm.FetchRequest{
			Method: "POST",
			URL:    cfg.URL + "/api/chat",
			Body:   reqBody,
		})
		if err != nil {
			return "", fmt.Errorf("ollama fetch failed: %w", err)
		}
		if resp.Status != 200 {
			return "", fmt.Errorf("ollama error: %d - %s", resp.Status, string(resp.Body))
		}
		var res struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(resp.Body, &res); err != nil {
			return "", fmt.Errorf("failed to parse ollama response: %w - %s", err, string(resp.Body))
		}
		return res.Message.Content, nil

	case "openai":
		reqBody, _ := json.Marshal(map[string]any{
			"model": cfg.Model,
			"messages": []map[string]string{
				{"role": "user", "content": fullPrompt},
			},
		})
		resp, err := wasm.Fetch(wasm.FetchRequest{
			Method: "POST",
			URL:    "https://api.openai.com/v1/chat/completions",
			Headers: map[string]string{
				"Authorization": "Bearer " + cfg.APIKey,
				"Content-Type":  "application/json",
			},
			Body: reqBody,
		})
		if err != nil {
			return "", fmt.Errorf("openai fetch failed: %w", err)
		}
		if resp.Status != 200 {
			return "", fmt.Errorf("openai error: %d - %s", resp.Status, string(resp.Body))
		}
		var res struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(resp.Body, &res); err != nil || len(res.Choices) == 0 {
			return "", fmt.Errorf("failed to parse openai response: %w", err)
		}
		return res.Choices[0].Message.Content, nil

	case "anthropic":
		reqBody, _ := json.Marshal(map[string]any{
			"model":      cfg.Model,
			"max_tokens": 1024,
			"messages": []map[string]string{
				{"role": "user", "content": fullPrompt},
			},
		})
		resp, err := wasm.Fetch(wasm.FetchRequest{
			Method: "POST",
			URL:    "https://api.anthropic.com/v1/messages",
			Headers: map[string]string{
				"x-api-key":         cfg.APIKey,
				"anthropic-version": "2023-06-01",
				"content-type":      "application/json",
			},
			Body: reqBody,
		})
		if err != nil {
			return "", fmt.Errorf("anthropic fetch failed: %w", err)
		}
		if resp.Status != 200 {
			return "", fmt.Errorf("anthropic error: %d - %s", resp.Status, string(resp.Body))
		}
		var res struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(resp.Body, &res); err != nil || len(res.Content) == 0 {
			return "", fmt.Errorf("failed to parse anthropic response: %w", err)
		}
		return res.Content[0].Text, nil

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
