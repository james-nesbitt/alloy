//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/build/gen/bindings/guest"
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	Id        string `json:"id"`
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
	// Create a new WIT-based plugin
	plugin = NewPlugin("ai").
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
	plugin.Handle("chat:message", handleChatMessageEvent)

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "AI Agent initializing")
		// Initialize with default config if none exists
		_, err := configStore.Get("current")
		if err != nil {
			defaultConfig := ProviderConfig{
				Type:  ProviderMock,
				Model: "mock-model",
			}
			_ = configStore.Set("current", defaultConfig)
		}
		return nil
	})

	// Set up event handling
	plugin.OnStart(func() {
		// Subscribe to chat events
		subPayload, _ := json.Marshal(map[string]string{"topic": "chat:message"})
		plugin.RouteMessage(AlloyMessage{
			MsgType: "request",
			Method:  "subscribe",
			Sender:  "ai",
			Target:  Some("events"),
			Payload: subPayload,
		})
	})

	// Run the plugin
	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

// handleConfigSet handles setting the AI provider configuration.
func handleConfigSet(msg AlloyMessage) AlloyMessage {
	var cfg ProviderConfig
	if err := json.Unmarshal(msg.Payload, &cfg); err != nil {
		return plugin.ErrorReply(msg, "invalid_config: "+err.Error())
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
		return plugin.ErrorReply(msg, "failed_to_save_config: "+err.Error())
	}

	return plugin.Reply(msg, map[string]interface{}{
		"status": "ok",
		"config": cfg,
	})
}

// handleConfigGet handles getting the current AI provider configuration.
func handleConfigGet(msg AlloyMessage) AlloyMessage {
	cfg, err := configStore.Get("current")
	if err != nil {
		// Return default config if none exists
		cfg = ProviderConfig{
			Type:  ProviderOllama,
			URL:   "http://127.0.0.1:11434",
			Model: "llama3",
		}
	}

	return plugin.Reply(msg, cfg)
}

// handleProviderSet handles setting the AI provider.
func handleProviderSet(msg AlloyMessage) AlloyMessage {
	var req ProviderSetRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_payload")
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
		return plugin.ErrorReply(msg, "failed_to_save_config")
	}

	return plugin.Reply(msg, map[string]interface{}{
		"status": "ok",
		"config": cfg,
	})
}

// handleQuery handles direct AI queries.
func handleQuery(msg AlloyMessage) AlloyMessage {
	var req AIQuery
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_query_payload")
	}

	// First try to find a native LLM provider
	nativeProvider := discoverNativeLLM()
	if nativeProvider != "" {
		response, err := queryNativeLLM(nativeProvider, req.Prompt)
		if err == nil {
			return plugin.Reply(msg, AIResponse{Response: response})
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
		return plugin.ErrorReply(msg, "query_failed: "+err.Error())
	}

	return plugin.Reply(msg, AIResponse{Response: response})
}

// handleSummarize handles text summarization requests.
func handleSummarize(msg AlloyMessage) AlloyMessage {
	// Get active project
	project, err := getActiveProject()
	if err != nil {
		return plugin.ErrorReply(msg, "no_active_project")
	}

	// Create summary prompt
	summaryPrompt := fmt.Sprintf("Summarize the following project:\nName: %s\nDescription: %s",
		project.Name, project.Description)

	// Query the AI
	response, err := performLLMQueryWithFallback(summaryPrompt)
	if err != nil {
		return plugin.ErrorReply(msg, "summarization_failed: "+err.Error())
	}

	return plugin.Reply(msg, ProjectSummary{Summary: response})
}

// discoverNativeLLM discovers native LLM providers.
func discoverNativeLLM() string {
	// Create discovery message
	discoverMsg := AlloyMessage{
		Id:      "ai-discover-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "request",
		Method:  "discover",
		Sender:  "ai",
		Target:  Some("command-manager"),
	}

	// Call the command manager
	resp := plugin.Call(discoverMsg)
	if resp.Id == "" {
		return ""
	}

	// Parse the response
	var discoveryResult struct {
		Targets []struct {
			Id           string `json:"id"`
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
				plugin.Log("info", "Found native LLM provider: "+target.Id)
				return target.Id
			}
		}
	}

	return ""
}

// queryNativeLLM queries a native LLM provider.
func queryNativeLLM(providerID, prompt string) (string, error) {
	// Create query message
	queryMsg := AlloyMessage{
		Id:      "ai-gen-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "request",
		Method:  "generate",
		Sender:  "ai",
		Target:  Some(providerID),
		Payload: json.RawMessage(`{"prompt":"` + prompt + `"}`),
	}

	// Call the provider
	resp := plugin.Call(queryMsg)
	if resp.Id == "" {
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
		response := "Mock AI response to: " + prompt
		if projectContext != "" {
			response = fmt.Sprintf("Mock AI response with project context [%s] to: %s", projectContext, prompt)
		}
		return response, nil
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

	_ = map[string]interface{}{
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
	msg := AlloyMessage{
		Id:      "get-active-project-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "request",
		Method:  "get_active",
		Sender:  "ai",
		Target:  Some("project"),
	}

	// Call the project manager
	resp := plugin.Call(msg)
	if resp.Id == "" {
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
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// handleChatMessageEvent handles chat message events delivered by the events service.
func handleChatMessageEvent(msg AlloyMessage) AlloyMessage {
	plugin.Log("info", fmt.Sprintf("AI received event: %s", msg.Method))
	var chatMsg ChatMessage
	if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
		plugin.Log("error", "failed to unmarshal chat message: "+err.Error())
		return AlloyMessage{}
	}

	plugin.Log("info", fmt.Sprintf("AI message from %s: %s", chatMsg.Sender, chatMsg.Content))

	// Skip messages from ourselves or the chat plugin
	if chatMsg.Sender == "ai" || chatMsg.Sender == "chat" {
		return AlloyMessage{}
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
	return AlloyMessage{}
}

// sendChatResponse sends a response to a chat channel.
func sendChatResponse(channel, response string) {
	// Create message to send chat response
	msg := AlloyMessage{
		Id:      "ai-response-" + fmt.Sprint(time.Now().UnixNano()),
		MsgType: "event",
		Method:  "send",
		Sender:  "ai",
		Target:  Some("chat"),
		Payload: json.RawMessage(fmt.Sprintf(`{"channel":"%s","content":"%s"}`, channel, response)),
	}

	// Route the message
	plugin.RouteMessage(msg)
}