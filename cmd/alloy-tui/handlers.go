package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
	"github.com/james-nesbitt/alloy/pkg/frontend/tui"
)

func (m *Model) processMessage(msg api.Message) tea.Cmd {
	var cmds []tea.Cmd
	var displayMsg string
	if msg.Sender == "events" && msg.Method == "project:opened" {
		var event struct {
			Topic string           `json:"topic"`
			Data  frontend.Project `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &event); err == nil {
			m.ActiveProject = &event.Data
			displayMsg = fmt.Sprintf("[%s] Project opened: %s", time.Now().Format("15:04:05"), m.ActiveProject.Name)

			// Apply multi-pane layout if defined
			if len(m.ActiveProject.Layout.Layout) > 0 {
				newPanes := []tui.Pane{}
				for _, lp := range m.ActiveProject.Layout.Layout {
					p := tui.Pane{WidthPct: lp.WidthPct}
					switch lp.Type {
					case "dashboard":
						p.Type = tui.ModeDashboard
					case "chat":
						p.Type = tui.ModeChat
					case "editor":
						p.Type = tui.ModeEdit
					default:
						p.Type = tui.ModeNormal
					}
					newPanes = append(newPanes, p)
				}
				m.Panes = newPanes
				m.FocusIdx = 0
				m.Mode = m.Panes[0].Type
			} else {
				if m.ActiveProject.Layout.DefaultMode == "dashboard" {
					m.Mode = tui.ModeDashboard
				} else if m.ActiveProject.Layout.DefaultMode == "chat" {
					m.Mode = tui.ModeChat
				}
				m.Panes = []tui.Pane{{Type: m.Mode, WidthPct: 1.0}}
				m.FocusIdx = 0
			}
		}
	}

	if msg.Sender == "events" && msg.Method == "workspace:opened" {
		var event struct {
			Topic string             `json:"topic"`
			Data  frontend.Workspace `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &event); err == nil {
			// If workspace has a custom layout, apply it
			if event.Data.Layout != "" {
				var wCfg api.WorkspaceConfig
				if err := json.Unmarshal([]byte(event.Data.Layout), &wCfg); err == nil {
					if len(wCfg.Layout) > 0 {
						newPanes := []tui.Pane{}
						for _, lp := range wCfg.Layout {
							p := tui.Pane{WidthPct: lp.WidthPct}
							switch lp.Type {
							case "dashboard":
								p.Type = tui.ModeDashboard
							case "chat":
								p.Type = tui.ModeChat
							case "editor":
								p.Type = tui.ModeEdit
							default:
								p.Type = tui.ModeNormal
							}
							newPanes = append(newPanes, p)
						}
						m.Panes = newPanes
						m.FocusIdx = 0
						m.Mode = m.Panes[0].Type
						displayMsg = fmt.Sprintf("[%s] Workspace layout applied: %s", time.Now().Format("15:04:05"), event.Data.Name)
					}
				}
			} else {
				displayMsg = fmt.Sprintf("[%s] Workspace active: %s", time.Now().Format("15:04:05"), event.Data.Name)
			}
		}
	}

	if msg.Sender == "events" && (msg.Method == "presence:online" || msg.Method == "presence:offline" || msg.Method == "presence:heartbeat") {
		var event struct {
			Topic string `json:"topic"`
			Data  struct {
				UserID    string `json:"user_id"`
				Event     string `json:"event"`
				Timestamp int64  `json:"timestamp"`
			} `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &event); err == nil {
			if event.Data.Event == "online" {
				displayMsg = fmt.Sprintf("[%s] User joined: %s", time.Now().Format("15:04:05"), event.Data.UserID)
			} else if event.Data.Event == "offline" {
				displayMsg = fmt.Sprintf("[%s] User left: %s", time.Now().Format("15:04:05"), event.Data.UserID)
			}
		}
	}

	if msg.Method == "read" && msg.Sender == "buffer" {
		var resp struct {
			ID      string                `json:"id"`
			Content []byte                `json:"content"`
			Version int                   `json:"version"`
			Cursors map[string]tui.Cursor `json:"cursors,omitempty"`
		}
		if err := json.Unmarshal(msg.Payload, &resp); err == nil {
			m.textarea.SetValue(string(resp.Content))
			m.localBufferVersion = resp.Version
			m.isLocalBufferDirty = false
			m.remoteCursors = resp.Cursors
		}
	}

	if msg.Method == "write" && msg.Sender == "buffer" {
		var resp struct {
			Status  string `json:"status"`
			Version int    `json:"version"`
		}
		if err := json.Unmarshal(msg.Payload, &resp); err == nil && resp.Status == "ok" {
			m.localBufferVersion = resp.Version
			m.isLocalBufferDirty = false
		}
	}

	if msg.Method == "buffer:update" {
		var bData struct {
			BufferID string `json:"buffer_id"`
			Event    string `json:"event"`
		}
		if err := json.Unmarshal(msg.Payload, &bData); err == nil {
			if bData.BufferID == m.activeBuffer {
				if bData.Event == "cursor_update" || bData.Event == "cursors_updated" {
					// We only want the cursors, and the buffer read gives them to us
					cmds = append(cmds, m.fetchBufferContent(m.activeBuffer))
				} else if !m.isLocalBufferDirty {
					// Auto-refetch if not dirty
					cmds = append(cmds, m.fetchBufferContent(m.activeBuffer))
				}
			}
		}
	}

	if displayMsg == "" && msg.Sender == "project" && msg.Method == "list-workspaces-resp" {
		var resp struct {
			Workspaces []frontend.Workspace `json:"workspaces"`
		}
		if err := json.Unmarshal(msg.Payload, &resp); err == nil {
			m.Workspaces = resp.Workspaces
		}
	}

	if displayMsg == "" && msg.Sender == "kernel" && msg.Method == "dashboard:list-widgets-resp" {
		var widgets []api.Widget
		if err := json.Unmarshal(msg.Payload, &widgets); err == nil {
			for _, w := range widgets {
				if m.DashboardTiles == nil {
					m.DashboardTiles = make(map[string]frontend.DashboardTile)
				}
				m.DashboardTiles[w.ID] = frontend.DashboardTile{
					ID:          w.ID,
					Title:       w.Title,
					ContentType: w.ContentType,
					RawContent:  w.Content,
					RefreshMS:   w.RefreshIntervalMs,
					Timestamp:   time.Now().Unix(),
					Content:     []string{string(w.Content)},
				}
				found := false
				for _, id := range m.TileOrder {
					if id == w.ID {
						found = true
						break
					}
				}
				if !found {
					m.TileOrder = append(m.TileOrder, w.ID)
				}
			}
			displayMsg = fmt.Sprintf("[%s] Dashboard synchronized (%d widgets)", time.Now().Format("15:04:05"), len(widgets))
		}
	}

	if displayMsg == "" && msg.Sender == "events" {
		switch msg.Method {
		case "plugin:crashed":
			var ev struct {
				Topic string `json:"topic"`
				Data  struct {
					ID    string `json:"id"`
					Error string `json:"error"`
				} `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &ev); err == nil {
				if m.statuses == nil {
					m.statuses = make(map[string]string)
				}
				m.statuses[ev.Data.ID] = "crashed"
				displayMsg = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render(
					fmt.Sprintf("[%s] !!! Plugin %s CRASHED: %s", time.Now().Format("15:04:05"), ev.Data.ID, ev.Data.Error))
			}
		case "plugin:load_failed":
			var ev struct {
				Topic string `json:"topic"`
				Data  struct {
					ID    string `json:"id"`
					Error string `json:"error"`
				} `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &ev); err == nil {
				displayMsg = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(
					fmt.Sprintf("[%s] !!! Plugin %s Load Failed: %s", time.Now().Format("15:04:05"), ev.Data.ID, ev.Data.Error))
			}
		case "chat:message":
			var chatMsg struct {
				Sender  string `json:"sender"`
				Channel string `json:"channel"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(msg.Payload, &chatMsg); err == nil {
				displayMsg = fmt.Sprintf("[%s] #%s <%s> %s",
					time.Now().Format("15:04:05"), chatMsg.Channel, chatMsg.Sender, chatMsg.Content)
			}
		case "component:registered":
			// NEW: React to new components by updating discovery
			cmds = append(cmds, m.doDiscovery)
			displayMsg = "skip"
		case "dashboard:widget-registered":
			var w api.Widget
			if err := json.Unmarshal(msg.Payload, &w); err == nil {
				if m.DashboardTiles == nil {
					m.DashboardTiles = make(map[string]frontend.DashboardTile)
				}
				m.DashboardTiles[w.ID] = frontend.DashboardTile{
					ID:          w.ID,
					Title:       w.Title,
					ContentType: w.ContentType,
					RawContent:  w.Content,
					RefreshMS:   w.RefreshIntervalMs,
					Timestamp:   time.Now().Unix(),
					Content:     []string{string(w.Content)}, // Fallback
				}
				found := false
				for _, id := range m.TileOrder {
					if id == w.ID {
						found = true
						break
					}
				}
				if !found {
					m.TileOrder = append(m.TileOrder, w.ID)
				}
				displayMsg = fmt.Sprintf("[%s] New dashboard widget: %s (%s)", time.Now().Format("15:04:05"), w.Title, msg.Sender)
			}
		case "dashboard:widget-updated":
			widgetID, _ := msg.Metadata["widget_id"].(string)
			if tile, ok := m.DashboardTiles[widgetID]; ok {
				tile.RawContent = msg.Payload
				tile.Content = []string{string(msg.Payload)}
				tile.Timestamp = time.Now().Unix()
				m.DashboardTiles[widgetID] = tile
			}
			displayMsg = "skip"
		case "dashboard:widget-unregistered":
			var dat struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(msg.Payload, &dat); err == nil {
				delete(m.DashboardTiles, dat.ID)
				newOrder := []string{}
				for _, id := range m.TileOrder {
					if id != dat.ID {
						newOrder = append(newOrder, id)
					}
				}
				m.TileOrder = newOrder
			}
		}
	}

	if displayMsg == "" {
		if msg.Type == api.TypeResponse {
			displayMsg = fmt.Sprintf("[%s] < %s", time.Now().Format("15:04:05"), string(msg.Payload))
		} else {
			displayMsg = fmt.Sprintf("[%s] %s: %s", time.Now().Format("15:04:05"), msg.Sender, string(msg.Payload))
		}
	}

	if displayMsg == "skip" {
		return tea.Batch(append(cmds, m.listenForMessages())...)
	}

	m.messages = append(m.messages, displayMsg)
	m.viewport.SetContent(strings.Join(m.messages, "\n"))
	m.viewport.GotoBottom()
	return tea.Batch(append(cmds, m.listenForMessages())...)
}

func (m Model) hasCapability(cap string) bool {
	if m.commandTree == nil {
		return false
	}
	// We check if any node in the tree provides this method
	flattened := m.commandTree.Flatten("")
	for _, item := range flattened {
		if item.Method == cap {
			return true
		}
	}
	return false
}

func (m Model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case ":", "alt+x":
		m.lastMainMode = m.Mode
		m.Mode = tui.ModeCommand
		m.isLeader = false
		m.commandInput.SetValue(":")
		m.commandInput.Focus()
		return m, nil
	case " ":
		m.lastMainMode = m.Mode
		m.Mode = tui.ModeCommand
		m.isLeader = true
		m.commandInput.SetValue("")
		m.commandInput.Focus()
		return m, nil
	case "i":
		if !m.hasCapability("buffer:write") && !m.hasCapability("ui:view:editor") {
			m.messages = append(m.messages, lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("!! Editor service (buffer) not available."))
			return m, nil
		}
		m.Mode = tui.ModeInsert
		m.textarea.Focus()
		return m, nil
	case "d":
		m.Mode = tui.ModeDashboard
		m.isLeader = false
		return m, nil
	case "v":
		m.Mode = tui.ModeNormal
		m.isLeader = false
		return m, nil
	case "c":
		if !m.hasCapability("chat:send") && !m.hasCapability("ui:view:chat") {
			m.messages = append(m.messages, lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("!! Chat service not available."))
			return m, nil
		}
		m.Mode = tui.ModeChat
		m.textarea.Placeholder = "Type message to #" + m.ActiveChannel + "..."
		m.textarea.Focus()
		return m, nil
	case "ctrl+c", "q":
		return m, tea.Quit
	case "tab":
		if len(m.Panes) > 1 {
			m.FocusIdx = (m.FocusIdx + 1) % len(m.Panes)
			m.Mode = m.Panes[m.FocusIdx].Type
		}
		return m, nil
	case "shift+tab":
		if len(m.Panes) > 1 {
			m.FocusIdx = (m.FocusIdx - 1 + len(m.Panes)) % len(m.Panes)
			m.Mode = m.Panes[m.FocusIdx].Type
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleInsertMode(msg tea.KeyMsg, tiCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.Mode = tui.ModeNormal
		m.textarea.Blur()
		return m, nil
	}
	return m, tiCmd
}

func (m Model) handleChatMode(msg tea.KeyMsg, tiCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.Mode = tui.ModeNormal
		m.textarea.Blur()
		return m, nil
	case tea.KeyEnter:
		content := m.textarea.Value()
		if content != "" {
			m.textarea.SetValue("")
			return m, m.sendChatMessage(content)
		}
	}
	return m, tiCmd
}

func (m Model) handleEditMode(msg tea.KeyMsg, tiCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.Mode = tui.ModeNormal
		m.textarea.Blur()
		return m, nil
	case tea.KeyCtrlS:
		// Manual save
		return m, m.sendBufferUpdate(m.activeBuffer, m.textarea.Value(), false)
	}
	return m, tiCmd
}

func (m Model) handleCommandMode(msg tea.KeyMsg, ciCmd tea.Cmd) (tea.Model, tea.Cmd) {
	input := m.commandInput.Value()

	if m.isLeader && msg.Type == tea.KeyRunes && input == "" {
		char := string(msg.Runes)
		if char == ":" {
			m.isLeader = false
			m.breadcrumbs = nil
			m.commandInput.SetValue(":")
			m.selectedCmdIdx = 0
			return m, nil
		}

		node := m.commandTree.Find(m.breadcrumbs)
		if node != nil {
			if child, ok := node.Children[char]; ok {
				if len(child.Children) == 0 {
					m.Mode = m.lastMainMode
					m.isLeader = false
					m.breadcrumbs = nil
					m.selectedCmdIdx = 0
					m.commandInput.Blur()
					m.commandInput.SetValue("")
					return m.executeCommand(fmt.Sprintf("%s %s", child.Target, child.Method))
				} else {
					m.breadcrumbs = append(m.breadcrumbs, char)
					m.selectedCmdIdx = 0
					m.commandInput.SetValue("")
					return m, nil
				}
			}
		}
	}

	switch msg.Type {
	case tea.KeyDown, tea.KeyCtrlN, tea.KeyUp, tea.KeyCtrlP, tea.KeyEnter, tea.KeyEsc, tea.KeyCtrlG:
	default:
		m.commandInput, ciCmd = m.commandInput.Update(msg)
		m.selectedCmdIdx = 0
		if m.isLeader && strings.HasPrefix(m.commandInput.Value(), ":") {
			m.isLeader = false
			m.breadcrumbs = nil
		}
	}

	filteredCount := len(m.filteredCommands())

	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlG:
		m.Mode = m.lastMainMode
		if m.Mode == tui.ModeCommand {
			m.Mode = tui.ModeNormal
		}
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		m.breadcrumbs = nil
		m.selectedCmdIdx = 0
		m.isLeader = false
		return m, nil
	case tea.KeyDown, tea.KeyCtrlN:
		if filteredCount > 0 {
			m.selectedCmdIdx = (m.selectedCmdIdx + 1) % filteredCount
		}
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		if filteredCount > 0 {
			m.selectedCmdIdx = (m.selectedCmdIdx - 1 + filteredCount) % filteredCount
		}
		return m, nil
	case tea.KeyEnter:
		filtered := m.filteredCommands()
		if len(filtered) > 0 && m.selectedCmdIdx >= 0 && m.selectedCmdIdx < len(filtered) {
			opt := filtered[m.selectedCmdIdx]
			if m.selectType == tui.SelectProject {
				m.Mode = m.lastMainMode
				m.commandInput.Blur()
				m.selectType = tui.SelectNone
				m.commandInput.SetValue("")
				m.isLeader = false
				return m.executeCommand(fmt.Sprintf("project open %s", opt.Raw))
			}
			if m.selectType == tui.SelectWorkspace {
				m.Mode = m.lastMainMode
				m.commandInput.Blur()
				m.selectType = tui.SelectNone
				m.commandInput.SetValue("")
				m.isLeader = false
				return m.executeCommand(fmt.Sprintf("project set-workspace %s", opt.Raw))
			}

			if m.isLeader && m.commandInput.Value() == "" {
				if opt.IsDir {
					m.breadcrumbs = append(m.breadcrumbs, opt.Display)
					m.selectedCmdIdx = 0
					m.commandInput.SetValue("")
					return m, nil
				} else {
					node := m.commandTree.Find(append(m.breadcrumbs, opt.Display))
					if node != nil {
						if len(node.Params) > 0 {
							m.Mode = tui.ModeForm
							m.formTitle = node.Target + " " + node.Method
							m.formFields = node.Params
							m.formValues = make([]string, len(node.Params))
							m.formIdx = 0
							m.commandInput.SetValue("")
							m.commandInput.Focus()
							return m, nil
						}
						m.Mode = m.lastMainMode
						m.commandInput.Blur()
						m.commandInput.SetValue("")
						m.breadcrumbs = nil
						m.isLeader = false
						return m.executeCommand(fmt.Sprintf("%s %s", node.Target, node.Method))
					}
				}
			} else {
				// Omni mode
				if len(opt.Params) > 0 {
					m.Mode = tui.ModeForm
					m.formTitle = opt.Raw
					m.formFields = opt.Params
					m.formValues = make([]string, len(opt.Params))
					m.formIdx = 0
					m.commandInput.SetValue("")
					m.commandInput.Focus()
					return m, nil
				}
				m.Mode = m.lastMainMode
				m.commandInput.Blur()
				m.commandInput.SetValue("")
				m.breadcrumbs = nil
				m.isLeader = false
				return m.executeCommand(opt.Raw)
			}
		}

		cmd := m.commandInput.Value()
		m.Mode = m.lastMainMode
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		m.breadcrumbs = nil
		m.isLeader = false
		return m.executeCommand(cmd)
	case tea.KeyBackspace:
		if m.commandInput.Value() == "" && m.isLeader {
			if len(m.breadcrumbs) > 0 {
				m.breadcrumbs = m.breadcrumbs[:len(m.breadcrumbs)-1]
				m.selectedCmdIdx = 0
				return m, nil
			} else {
				m.isLeader = false
				m.commandInput.SetValue(":")
				return m, nil
			}
		}
	}

	return m, ciCmd
}

func (m Model) handleFormMode(msg tea.KeyMsg, ciCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlG:
		m.Mode = tui.ModeNormal
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		return m, nil
	case tea.KeyEnter:
		m.formValues[m.formIdx] = m.commandInput.Value()
		m.formIdx++
		if m.formIdx >= len(m.formFields) {
			m.Mode = tui.ModeNormal
			m.commandInput.Blur()
			m.commandInput.SetValue("")
			return m.executeFormCommand()
		}
		m.commandInput.SetValue("")
		return m, nil
	default:
		m.commandInput, ciCmd = m.commandInput.Update(msg)
	}
	return m, ciCmd
}
