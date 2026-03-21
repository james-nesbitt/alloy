//go:build wasip1 || wasm

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jnesbitt/alloy-go/pkg/wasm2/guest"
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
	Name    string        `json:"name"`
	APIKey  string        `json:"api_key,omitempty"`
	URL     string        `json:"url"`
	Model   string        `json:"model"`
	Type    AIProviderType `json:"type"` // "openai", "anthropic", "ollama"
}

// ChatMessage represents a chat message.
type ChatMessage struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// AIQuery represents a query to the AI.
type AIQuery struct {
	Prompt string `json:"prompt"`
}

// AIResponse represents a response from the AI.
type AIResponse struct {
	Response string `json:"response"`
}

// ProviderSetRequest represents a request to set the AI provider.
type ProviderSetRequest struct {
	Type  string `json:"type"`
	Model string `json:"model,omitempty"`
	URL   string `json:"url,omitempty"`
}

// ProjectSummary represents a project summary.
type ProjectSummary struct {
	Summary string `json:"summary"`
}

var (
	plugin      *guest.Plugin
	configStore = NewKVStore[ProviderConfig]("ai-agent:config")
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
	// Create a new WIT-based plugin
	plugin = guest.NewPlugin("ai-agent").
		WithMetadata(
			"AI Agent", 
			"Provides AI capabilities including chat and summarization",
			"0.1.0", 
			"Alloy Team",
		).
		WithTags("ai", "llm", "chatbot", "automation").
		WithCapability("config:set", "Configure AI provider").
		WithCapability("config:get", "Get current configuration").
		WithCapability("provider:set", "Switch AI provider").
		WithCapability("query", "Query the AI directly").
		WithCapability("summarize", "Summarize provided text")

	// Set up message handlers
	plugin.Handle("config:set", handleConfigSet)
	plugin.Handle("config:get", handleConfigGet)
	plugin.Handle("provider:set", handleProviderSet)
	plugin.Handle("query", handleQuery)
	plugin.Handle("summarize", handleSummarize)

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "AI Agent initializing")
		// Initialize with default config if none exists
		_, err := configStore.Get("current")
		if err != nil {
			defaultConfig := ProviderConfig{
				Type:  ProviderOllama,
				URL:   "http://127.0.0.1:11434",
				Model: "llama3",
			}
			_ = configStore.Set("current", defaultConfig)
		}
		return nil
	})

	// Set up event handling
	plugin.OnStart(func() {
		// Subscribe to chat events
		plugin.RouteMessage(guest.AlloyMessage{
			Method: "subscribe",
			Sender: "ai-agent",
			Target: guest.AlloyOption[string]{Value: "plugin-events", Set: true},
			Payload: json.RawMessage(`{"event":"chat:message"}`),
		})
	})

	// Run the plugin
	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

// handleConfigSet handles setting the AI provider configuration.
func handleConfigSet(msg guest.AlloyMessage) guest.AlloyMessage {
	var cfg ProviderConfig
	if err := json.Unmarshal(msg.Payload, &cfg); err != nil {
		return guest.ErrorReply(msg, "invalid_config: "+err.Error())
	}

	// Validate and set defaults
	if cfg.Type == "" {
		cfg.Type = ProviderOllama
	}
	if cfg.URL == "" && cfg.Type == ProviderOllama {
		cfg.URL = "http://127.0.0.1:11434"
	}
	if cfg.Model == "" {
		switch cfg.Type {
		case ProviderOllama:
			cfg.Model = "llama3"
		case ProviderOpenAI:
			cfg.Model = "gpt-4o"
		case ProviderAnthropic:
			cfg.Model = "claude-3-5-sonnet"
		}
	}

	// Save the configuration
	if err := configStore.Set("current", cfg); err != nil {
		return guest.ErrorReply(msg, "failed_to_save_config: "+err.Error())
	}

	return guest.Reply(msg, map[string]interface{}{
		"status": "ok",
		"config": cfg,
	})
}

// handleConfigGet handles getting the current AI provider configuration.
func handleConfigGet(msg guest.AlloyMessage) guest.AlloyMessage {
	cfg, err := configStore.Get("current")
	if err != nil {
		// Return default config if none exists
		cfg = ProviderConfig{
			Type:  ProviderOllama,
			URL:   "http://127.0.0.1:11434",
			Model: "llama3",
		}
	}

	return guest.Reply(msg, cfg)
}

// handleProviderSet handles setting the AI provider.
func handleProviderSet(msg guest.AlloyMessage) guest.AlloyMessage {
	var req ProviderSetRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return guest.ErrorReply(msg, "invalid_payload")
	}

	cfg, err := configStore.Get("current")
	if err != nil {
		cfg = ProviderConfig{Type: ProviderOllama}
	}

	cfg.Type = AIProviderType(req.Type)
	if req.Model != "" {
		cfg.Model = req.Model
	}
	if req.URL != "" {
		cfg.URL = req.URL
	}

	// Set defaults if missing
	if cfg.URL == "" && cfg.Type == ProviderOllama {
		cfg.URL = "http://127.0.0.1:11434"
	}
	if cfg.Model == "" {
		switch cfg.Type {
		case ProviderOllama:
			cfg.Model = "llama3"
		case ProviderOpenAI:
			cfg.Model = "gpt-4o"
		case ProviderAnthropic:
			cfg.Model = "claude-3-5-sonnet"
		}
	}

	// Save the configuration
	if err := configStore.Set("current", cfg); err != nil {
		return guest.ErrorReply(msg, "failed_to_save_config")
	}

	return guest.Reply(msg, map[string]interface{}{
		"status": "ok",
		"config": cfg,
	})
}

// handleQuery handles direct AI queries.
func handleQuery(msg guest.AlloyMessage) guest.AlloyMessage {
	var req AIQuery
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return guest.ErrorReply(msg, "invalid_query_payload")
	}

	// First try to find a native LLM provider
	nativeProvider := discoverNativeLLM()
	if nativeProvider != "" {
		response, err := queryNativeLLM(nativeProvider, req.Prompt)
		if err == nil {
			return guest.Reply(msg, AIResponse{Response: response})
		}
		plugin.Log("warn", fmt.Sprintf("Native LLM query failed: %v", err))
	}

	// Fall back to configured provider
	cfg, err := configStore.Get("current")
	if err != nil {
		cfg = ProviderConfig{Type: ProviderMock, Model: "test-model"}
	}

	// Get project context
	projectContext := getProjectContext()

	// Combine prompt with context
	fullPrompt := req.Prompt
	if projectContext != "" {
		fullPrompt = projectContext + "\nUser Question: " + req.Prompt
	}

	// Query the AI provider
	response, err := performLLMQuery(cfg, fullPrompt)
	if err != nil {
		return guest.ErrorReply(msg, "query_failed: "+err.Error())
	}

	return guest.Reply(msg, AIResponse{Response: response})
}

// handleSummarize handles text summarization requests.
func handleSummarize(msg guest.AlloyMessage) guest.AlloyMessage {
	// Get active project
	project, err := getActiveProject()
	if err != nil {
		return guest.ErrorReply(msg, "no_active_project")
	}

	// Create summary prompt
	summaryPrompt := fmt.Sprintf("Summarize the following project:\nName: %s\nDescription: %s",
		project.Name, project.Description)

	// Query the AI
	response, err := performLLMQueryWithFallback(summaryPrompt)
	if err != nil {
		return guest.ErrorReply(msg, "summarization_failed: "+err.Error())
	}

	return guest.Reply(msg, ProjectSummary{Summary: response})
}

// discoverNativeLLM discovers native LLM providers.
func discoverNativeLLM() string {
	// Create discovery message
	discoverMsg := guest.AlloyMessage{
		ID:     "ai-discover-" + fmt.Sprint(time.Now().UnixNano()),
		Method: "discover",
		Sender: "ai-agent",
		Target: guest.AlloyOption[string]{Value: "plugin-command-manager", Set: true},
	}

	// Call the command manager
	resp, ok := plugin.Call(discoverMsg)
	if !ok {
		return ""
	}

	// Parse the response
	var discoveryResult struct {
		Targets []struct {
			ID           string `json:"id"`
			Capabilities []struct {
				Method      string            `json:"method"`
				Annotations map[string]string `json:"annotations"`
			} `json:"capabilities"`
		} `json:"targets"`
	}

	if err := json.Unmarshal(resp.Payload, &discoveryResult); err != nil {
		return ""
	}

	// Find LLM providers
	for _, target := range discoveryResult.Targets {
		for _, cap := range target.Capabilities {
			if cap.Method == "generate" && cap.Annotations["type"] == "llm" {
				plugin.Log("info", "Found native LLM provider: "+target.ID)
				return target.ID
			}
		}
	}

	return ""
}

// queryNativeLLM queries a native LLM provider.
func queryNativeLLM(providerID, prompt string) (string, error) {
	// Create query message
	queryMsg := guest.AlloyMessage{
		ID:      "ai-gen-" + fmt.Sprint(time.Now().UnixNano()),
		Method:  "generate",
		Sender:  "ai-agent",
		Target:  guest.AlloyOption[string]{Value: providerID, Set: true},
		Payload: json.RawMessage(`{"prompt":"` + prompt + `"}`),
	}

	// Call the provider
	resp, ok := plugin.Call(queryMsg)
	if !ok {
		return "", fmt.Errorf("call_failed")
	}

	// Parse the response
	var result struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		return "", err
	}

	return result.Response, nil
}

// performLLMQueryWithFallback performs an LLM query with fallback to mock.
func performLLMQueryWithFallback(prompt string) (string, error) {
	// First try to use the configured provider
	cfg, err := configStore.Get("current")
	if err == nil {
		return performLLMQuery(cfg, prompt)
	}

	// Fall back to mock response
	plugin.Log("warn", "Using mock AI provider")
	return "Mock AI response to: " + prompt, nil
}

// performLLMQuery performs an LLM query using the configured provider.
func performLLMQuery(cfg ProviderConfig, prompt string) (string, error) {
	// Get project context
	projectContext := getProjectContext()
	fullPrompt := prompt
	if projectContext != "" {
		fullPrompt = projectContext + "\nUser Question: " + prompt
	}

	// Query the provider
	switch cfg.Type {
	case ProviderMock:
		return "Mock AI response to: " + prompt, nil
	case ProviderOllama:
		return queryOllama(cfg, fullPrompt)
	case ProviderOpenAI:
		return queryOpenAI(cfg, fullPrompt)
	case ProviderAnthropic:
		return queryAnthropic(cfg, fullPrompt)
	default:
		return "", fmt.Errorf("unsupported_provider: %s", cfg.Type)
	}
}

// queryOllama queries the Ollama API.
func queryOllama(cfg ProviderConfig, prompt string) (string, error) {
	type OllamaResponse struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	request := map[string]interface{}{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream": false,
	}

	// In a real implementation, we would use the host's fetch capability
	// For now, we'll simulate it
	plugin.Log("info", fmt.Sprintf("Querying Ollama: %s/model=%s", cfg.URL, cfg.Model))

	// Simulate API call
	response := OllamaResponse{}
	mockContent := "This is a mock response from Ollama for: " + prompt
	response.Message.Content = mockContent

	return response.Message.Content, nil
}

// queryOpenAI queries the OpenAI API.
func queryOpenAI(cfg ProviderConfig, prompt string) (string, error) {
	type OpenAIResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	// In a real implementation, we would use the host's fetch capability
	plugin.Log("info", fmt.Sprintf("Querying OpenAI: model=%s", cfg.Model))

	// Simulate API call
	response := OpenAIResponse{}
	response.Choices = append(response.Choices, struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}{
		Message: struct {
			Content string `json:"content"`
		}{
			Content: "This is a mock response from OpenAI for: " + prompt,
		},
	})

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("openai: no_choices")
	}
	return response.Choices[0].Message.Content, nil
}

// queryAnthropic queries the Anthropic API.
func queryAnthropic(cfg ProviderConfig, prompt string) (string, error) {
	type AnthropicResponse struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}

	// In a real implementation, we would use the host's fetch capability
	plugin.Log("info", fmt.Sprintf("Querying Anthropic: model=%s", cfg.Model))

	// Simulate API call
	response := AnthropicResponse{}
	response.Content = append(response.Content, struct {
		Text string `json:"text"`
	}{
		Text: "This is a mock response from Anthropic for: " + prompt,
	})

	if len(response.Content) == 0 {
		return "", fmt.Errorf("anthropic: no_content")
	}
	return response.Content[0].Text, nil
}

// getProjectContext gets the current project context.
func getProjectContext() string {
	project, err := getActiveProject()
	if err != nil {
		return ""
	}

	return fmt.Sprintf("You are currently working on project '%s' (%s).",
		project.Name, project.Description)
}

// getActiveProject gets the active project.
func getActiveProject() (*ProjectInfo, error) {
	// Create message to get active project
	msg := guest.AlloyMessage{
		ID:     "get-active-project-" + fmt.Sprint(time.Now().UnixNano()),
		Method: "get_active",
		Sender: "ai-agent",
		Target: guest.AlloyOption[string]{Value: "plugin-project-manager", Set: true},
	}

	// Call the project manager
	resp, ok := plugin.Call(msg)
	if !ok {
		return nil, fmt.Errorf("failed_to_get_project")
	}

	// Parse the response
	var project ProjectInfo
	if err := json.Unmarshal(resp.Payload, &project); err != nil {
		return nil, err
	}

	return &project, nil
}

// ProjectInfo represents project information.
type ProjectInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// handleEvent handles incoming events.
func handleEvent(msg guest.AlloyMessage) {
	// This would be called when an event is received
	if msg.Method == "event" && len(msg.Payload) > 0 {
		var event struct {
			Event   string          `json:"event"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(msg.Payload, &event); err == nil {
			if event.Event == "chat:message" {
				handleChatMessage(event.Payload)
			}
		}
	}
}

// handleChatMessage handles chat messages.
func handleChatMessage(payload json.RawMessage) {
	var chatMsg ChatMessage
	if err := json.Unmarshal(payload, &chatMsg); err != nil {
		return
	}

	// Skip messages from ourselves or the chat plugin
	if chatMsg.Sender == "ai-agent" || chatMsg.Sender == "plugin-chat" {
		return
	}

	// Check if the message is an AI command
	if strings.HasPrefix(strings.ToLower(chatMsg.Content), "ai:") {
		prompt := strings.TrimSpace(chatMsg.Content[3:])
		response, err := performLLMQueryWithFallback(prompt)
		if err != nil {
			plugin.Log("error", "AI query failed: "+err.Error())
			sendChatResponse(chatMsg.Channel, "AI Error: "+err.Error())
		} else {
			sendChatResponse(chatMsg.Channel, response)
		}
	}
}

// sendChatResponse sends a response to a chat channel.
func sendChatResponse(channel, response string) {
	// Create message to send chat response
	msg := guest.AlloyMessage{
		ID:      "ai-response-" + fmt.Sprint(time.Now().UnixNano()),
		Method:  "send",
		Sender:  "ai-agent",
		Target:  guest.AlloyOption[string]{Value: "plugin-chat", Set: true},
		Payload: json.RawMessage(fmt.Sprintf(`{"channel":"%s","content":"%s"}`, channel, response)),
	}

	// Route the message
	plugin.RouteMessage(msg)
}