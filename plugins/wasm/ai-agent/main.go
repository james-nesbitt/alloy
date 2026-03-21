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

var (
	configStore = wasm.NewKVStore[ProviderConfig]("ai-agent:config")
)

func main() {
	var p *wasm.Plugin
	p = wasm.New("plugin-ai-agent").
		WithCapability("config:set", "Configure AI provider (JSON)", "a c s").
		WithCapability("config:get", "Get current configuration", "a c g").
		WithCapability("provider:set", "Switch AI provider (ollama|openai|anthropic)", "a p s").
		WithCapability("query", "Query the AI directly", "a q").
		WithCapability("summarize", "Summarize provided text Content", "a s").
		WithCapability("chat:message", "Reactive chat handler", ""). // internal
		OnInit(func() error {
			// Subscribe to chat events
			p.Events.Subscribe("chat:message")
			return nil
		})

	p.Handle("config:set", func(msg wasm.Message) wasm.Message {
		var cfg ProviderConfig
		if err := json.Unmarshal(msg.Payload, &cfg); err != nil {
			return wasm.ErrorReply(msg, "invalid config: "+err.Error())
		}
		// Validate config
		if cfg.Type == "" { cfg.Type = "ollama" }
		if cfg.URL == "" && cfg.Type == "ollama" { cfg.URL = "http://127.0.0.1:11434" }
		if cfg.Model == "" {
			switch cfg.Type {
			case "ollama": cfg.Model = "llama3"
			case "openai": cfg.Model = "gpt-4o"
			case "anthropic": cfg.Model = "claude-3-5-sonnet"
			}
		}

		_ = configStore.Set("current", cfg)
		return wasm.Reply(msg, map[string]any{"status": "ok", "config": cfg})
	})

	p.Handle("config:get", func(msg wasm.Message) wasm.Message {
		cfg, err := configStore.Get("current")
		if err != nil {
			cfg = ProviderConfig{Type: "ollama", URL: "http://127.0.0.1:11434", Model: "llama3"}
		}
		return wasm.Reply(msg, cfg)
	})

	p.Handle("provider:set", func(msg wasm.Message) wasm.Message {
		var req struct {
			Type  string `json:"type"`
			Model string `json:"model,omitempty"`
			URL   string `json:"url,omitempty"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return wasm.ErrorReply(msg, "invalid payload")
		}

		cfg, _ := configStore.Get("current")
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

		_ = configStore.Set("current", cfg)
		return wasm.Reply(msg, map[string]any{"status": "ok", "config": cfg})
	})

	p.Handle("query", func(msg wasm.Message) wasm.Message {
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.Unmarshal(msg.Payload, &req)

		response, err := performLLMQuery(p, req.Prompt)
		if err != nil {
			return wasm.ErrorReply(msg, err.Error())
		}
		return wasm.Reply(msg, map[string]string{"response": response})
	})

	p.Handle("chat:message", func(msg wasm.Message) wasm.Message {
		var chatMsg ChatMessage
		if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
			return wasm.Message{}
		}

		if chatMsg.Sender == "plugin-ai-agent" || chatMsg.Sender == "plugin-chat" {
			return wasm.Message{}
		}

		if strings.HasPrefix(strings.ToLower(chatMsg.Content), "ai:") {
			prompt := strings.TrimSpace(chatMsg.Content[3:])
			response, err := performLLMQuery(p, prompt)
			if err != nil {
				p.Chat.SendMessage(chatMsg.Channel, "AI Error: "+err.Error())
			} else {
				p.Chat.SendMessage(chatMsg.Channel, response)
			}
		}
		return wasm.Message{}
	})

	p.Handle("summarize", func(msg wasm.Message) wasm.Message {
		proj, err := p.Projects.GetActive()
		if err != nil {
			return wasm.ErrorReply(msg, "no active project to summarize")
		}

		summaryPrompt := fmt.Sprintf("Summarize the following project:\nName: %s\nDescription: %s",
			proj.Name, proj.Description)

		response, err := performLLMQuery(p, summaryPrompt)
		if err != nil {
			return wasm.ErrorReply(msg, err.Error())
		}

		return wasm.Reply(msg, map[string]string{"summary": response})
	})

	p.Run()
}

func discoverNativeLLM() string {
	resp, err := wasm.Call(wasm.Message{
		ID:     "ai-discover-" + fmt.Sprint(time.Now().UnixNano()),
		Type:   "request",
		Sender: "plugin-ai-agent",
		Target: "plugin-command-manager",
		Method: "discover",
	})
	if err != nil {
		return ""
	}

	var d struct {
		Targets []struct {
			ID           string            `json:"id"`
			Capabilities []wasm.Capability `json:"capabilities"`
		} `json:"targets"`
	}
	json.Unmarshal(resp.Payload, &d)

	for _, t := range d.Targets {
		for _, cap := range t.Capabilities {
			if cap.Method == "generate" && cap.Annotations["type"] == "llm" {
				wasm.Log("Found native LLM provider: " + t.ID)
				return t.ID
			}
		}
	}
	return ""
}

func performLLMQuery(p *wasm.Plugin, prompt string) (string, error) {
	// Try discovery of native providers first
	nativeID := discoverNativeLLM()
	if nativeID != "" {
		wasm.Log("Using native LLM provider: " + nativeID)
		resp, err := wasm.Call(wasm.Message{
			ID:     "ai-gen-" + fmt.Sprint(time.Now().UnixNano()),
			Type:   "request",
			Sender: "plugin-ai-agent",
			Target: nativeID,
			Method: "generate",
			Payload: json.RawMessage(`{"prompt":"` + prompt + `"}`),
		})
		if err == nil {
			var r struct {
				Response string `json:"response"`
			}
			json.Unmarshal(resp.Payload, &r)
			return r.Response, nil
		}
		wasm.Log("Native LLM call failed, falling back to configured API: " + err.Error())
	}

	cfg, err := configStore.Get("current")
	if err != nil {
		cfg = ProviderConfig{Type: "mock", Model: "test-model"}
	}

	// Fetch project context using high-level client
	var projectContext string
	if proj, err := p.Projects.GetActive(); err == nil {
		projectContext = fmt.Sprintf("\nYou are currently working on project '%s' (%s).\n",
			proj.Name, proj.Description)
	}

	fullPrompt := prompt
	if projectContext != "" {
		fullPrompt = projectContext + "\nUser Question: " + prompt
	}

	// Generic LLM fetch logic using high-level PostJSON
	switch cfg.Type {
	case "mock":
		return "Mock AI response to: " + prompt, nil
	case "ollama":
		type ollamaRes struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}
		req := map[string]any{
			"model": cfg.Model,
			"messages": []map[string]string{
				{"role": "user", "content": fullPrompt},
			},
			"stream": false,
		}
		res, err := wasm.PostJSON[any, ollamaRes](cfg.URL+"/api/chat", nil, req)
		if err != nil { return "", err }
		return res.Message.Content, nil

	case "openai":
		type openaiRes struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		req := map[string]any{
			"model": cfg.Model,
			"messages": []map[string]string{
				{"role": "user", "content": fullPrompt},
			},
		}
		headers := map[string]string{"Authorization": "Bearer " + cfg.APIKey}
		res, err := wasm.PostJSON[any, openaiRes]("https://api.openai.com/v1/chat/completions", headers, req)
		if err != nil { return "", err }
		if len(res.Choices) == 0 { return "", fmt.Errorf("openai: no choices") }
		return res.Choices[0].Message.Content, nil

	case "anthropic":
		type anthropicRes struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		req := map[string]any{
			"model":      cfg.Model,
			"max_tokens": 1024,
			"messages": []map[string]string{
				{"role": "user", "content": fullPrompt},
			},
		}
		headers := map[string]string{
			"x-api-key":         cfg.APIKey,
			"anthropic-version": "2023-06-01",
		}
		res, err := wasm.PostJSON[any, anthropicRes]("https://api.anthropic.com/v1/messages", headers, req)
		if err != nil { return "", err }
		if len(res.Content) == 0 { return "", fmt.Errorf("anthropic: no content") }
		return res.Content[0].Text, nil

	default:
		return "", fmt.Errorf("unsupported provider: %s", cfg.Type)
	}
}
