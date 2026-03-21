//go:build tinygo || wasip1 || wasm
package wasm

import (
	"encoding/json"
	"fmt"
	"time"
)

// Project is a high-level representation of a project metadata.
type Project struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
}

// ProjectClient provides high-level access to project management.
type ProjectClient struct {
	pluginID string
}

func NewProjectClient(pluginID string) *ProjectClient {
	return &ProjectClient{pluginID: pluginID}
}

// GetActive retrieves the current active project.
func (c *ProjectClient) GetActive() (*Project, error) {
	// Active project is typically saved in the shared KV under 'shared:active-project'
	data := KVGet("shared:active-project")
	if data == nil {
		return nil, fmt.Errorf("no active project")
	}
	var p Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ChatClient provides high-level access to the chat system.
type ChatClient struct {
	pluginID string
}

func NewChatClient(pluginID string) *ChatClient {
	return &ChatClient{pluginID: pluginID}
}

// SendMessage sends a message to a specific room/channel.
func (c *ChatClient) SendMessage(channel, content string) bool {
	payload, _ := json.Marshal(map[string]string{
		"channel": channel,
		"content": content,
	})
	msg := Message{
		ID:        fmt.Sprintf("chat-%d", time.Now().UnixNano()),
		Type:      "request",
		Sender:    c.pluginID,
		Target:    "plugin-chat",
		Method:    "send",
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}
	return RouteMessage(msg)
}

// SendDM sends a direct message to a specific user.
func (c *ChatClient) SendDM(to, content string) bool {
	payload, _ := json.Marshal(map[string]string{
		"to":      to,
		"content": content,
	})
	msg := Message{
		ID:        fmt.Sprintf("dm-%d", time.Now().UnixNano()),
		Type:      "request",
		Sender:    c.pluginID,
		Target:    "plugin-chat",
		Method:    "dm",
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}
	return RouteMessage(msg)
}

// IAMClient provides access to identity and access status.
type IAMClient struct {
	pluginID string
}

func NewIAMClient(pluginID string) *IAMClient {
	return &IAMClient{pluginID: pluginID}
}

// GetCurrentUser returns the user associated with the current session.
func (c *IAMClient) GetCurrentUser() string {
	// Session user is typically provided in shared memory/KV for current execution context
	data := KVGet("shared:session-user")
	if data == nil {
		return "anonymous"
	}
	return string(data)
}
