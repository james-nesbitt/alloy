package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

func (m Model) fetchBufferContent(id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		payload, _ := json.Marshal(map[string]string{"id": id})
		resp, err := m.client.Send(ctx, "buffer:read", "read", payload)
		if err != nil {
			return errMsg(err)
		}
		return messageMsg(resp)
	}
}

func (m Model) sendBufferUpdate(id string, content string, force bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		payload, _ := json.Marshal(map[string]any{
			"id":           id,
			"content":      content,
			"base_version": m.localBufferVersion,
			"force":        force,
		})
		resp, err := m.client.Send(ctx, "buffer:write", "write", payload)
		if err != nil {
			return errMsg(err)
		}

		if resp.Method == "error" {
			var errData struct {
				Error string `json:"error"`
			}
			json.Unmarshal(resp.Payload, &errData)
			if errData.Error == "conflict_detected" {
				return errMsg(fmt.Errorf("CONFLICT DETECTED: Someone else has modified this buffer. Use :force-save to overwrite."))
			}
		}

		return messageMsg(resp)
	}
}

func (m Model) sendCursorUpdate(id string, row, col int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		payload, _ := json.Marshal(map[string]any{
			"id":  id,
			"row": row,
			"col": col,
		})
		_, _ = m.client.Send(ctx, "buffer:update-cursor", "update_cursor", payload)
		return nil
	}
}

func (m Model) subscribe(topic string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		payload, _ := json.Marshal(map[string]string{"topic": topic})
		_, _ = m.client.Send(ctx, "events:subscribe", "subscribe", payload)
		return nil
	}
}

func (m Model) sendChatMessage(content string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		var target string
		var method string
		var payload []byte

		if m.ActiveChannel != "" && (m.ActiveChannel[0:3] == "dm:") {
			target = "chat:direct:send"
			method = "direct:send"
			payload, _ = json.Marshal(map[string]string{
				"to":      m.ActiveChannel[3:],
				"content": content,
			})
		} else {
			target = "chat:send"
			method = "send"
			payload, _ = json.Marshal(map[string]string{
				"channel": m.ActiveChannel,
				"content": content,
			})
		}

		_, err := m.client.Send(ctx, target, method, payload)
		if err != nil {
			return errMsg(err)
		}
		return nil
	}
}

func (m Model) sendPresenceHeartbeat() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		payload, _ := json.Marshal(map[string]string{
			"status": "online",
		})
		_, _ = m.client.Send(ctx, "chat:presence:update", "presence:update", payload)

		presence := frontend.Presence{
			User:     m.client.Actor(),
			Status:   "online",
			Client:   "tui",
			LastSeen: time.Now().Unix(),
		}
		if m.ActiveProject != nil {
			presence.ProjectID = m.ActiveProject.ID
		}

		eventData, _ := json.Marshal(map[string]any{
			"topic": "presence:heartbeat",
			"data":  presence,
		})
		_, _ = m.client.Send(ctx, "events:publish", "publish", eventData)

		return nil
	}
}

func (m Model) fetchActiveProject() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, _ := m.client.Send(ctx, "project:get_active", "get_active", nil)
		if resp.ID != "" {
			resp.Method = "active-resp"
			return messageMsg(resp)
		}
		return nil
	}
}

func (m Model) fetchProjects() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, _ := m.client.Send(ctx, "project:list", "list", nil)
		if resp.ID != "" {
			resp.Method = "list-resp"
			return messageMsg(resp)
		}
		return nil
	}
}

func (m Model) fetchWorkspaces() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, _ := m.client.Send(ctx, "project:list-workspaces", "list-workspaces", nil)
		if resp.ID != "" {
			resp.Method = "list-workspaces-resp"
			return messageMsg(resp)
		}
		return nil
	}
}

func (m Model) fetchInitialWidgets() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, _ := m.client.Send(ctx, "kernel", "dashboard:list-widgets", nil)
		if resp.ID != "" {
			resp.Method = "dashboard:list-widgets-resp"
			return messageMsg(resp)
		}
		return nil
	}
}
