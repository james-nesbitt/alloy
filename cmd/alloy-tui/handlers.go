package main

import (
	"context"
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
			if m.ActiveProject.Layout.Root != nil {
				m.RootLayout = m.ActiveProject.Layout.Root
				panes := tui.GetPanes(m.RootLayout)
				if len(panes) > 0 {
					m.FocusedPaneID = panes[0].ID
					// Sync mode with focused pane
					m.syncModeWithFocusedPane()
				}
			} else {
				mode := "dashboard"
				if m.ActiveProject.Layout.DefaultMode != "" {
					mode = m.ActiveProject.Layout.DefaultMode
				}
				m.RootLayout = &frontend.LayoutNode{
					ID:   "main-pane",
					Type: "pane",
					Mode: mode,
				}
				m.FocusedPaneID = "main-pane"
				m.syncModeWithFocusedPane()
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
				var wCfg frontend.WorkspaceConfig
				if err := json.Unmarshal([]byte(event.Data.Layout), &wCfg); err == nil {
					if wCfg.Root != nil {
						m.RootLayout = wCfg.Root
						panes := tui.GetPanes(m.RootLayout)
						if len(panes) > 0 {
							m.FocusedPaneID = panes[0].ID
							m.syncModeWithFocusedPane()
							displayMsg = fmt.Sprintf("[%s] Workspace layout applied: %s", time.Now().Format("15:04:05"), event.Data.Name)
						}
					}
				}
			} else {
				displayMsg = fmt.Sprintf("[%s] Workspace active: %s", time.Now().Format("15:04:05"), event.Data.Name)
			}
			// Load View State if available
			if event.Data.ViewState != "" {
				var vs map[string]interface{}
				if err := json.Unmarshal([]byte(event.Data.ViewState), &vs); err == nil {
					if id, ok := vs["focused_pane_id"].(string); ok {
						m.FocusedPaneID = id
					}
					if mode, ok := vs["last_mode"].(float64); ok {
						m.Mode = int(mode)
					}
					if ch, ok := vs["active_channel"].(string); ok {
						m.ActiveChannel = ch
					}
					if buf, ok := vs["active_buffer"].(string); ok {
						m.activeBuffer = buf
						if m.activeBuffer != "" {
							cmds = append(cmds, m.fetchBufferContent(m.activeBuffer))
						}
					}
					m.syncModeWithFocusedPane()
				}
			}
		}
	}

	if msg.Sender == "events" && msg.Method == "system:context-changed" {
		var event struct {
			Topic string `json:"topic"`
			Data  struct {
				ContextID string `json:"context_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &event); err == nil {
			// Trigger re-discovery with new context
			m.client.SetContext(event.Data.ContextID)

			displayMsg = fmt.Sprintf("[%s] Context changed: %s. Refreshing...", time.Now().Format("15:04:05"), event.Data.ContextID)

			// Get composed workspace from project plugin
			go func() {
				time.Sleep(200 * time.Millisecond) // Give system time to settle
				kMsg := api.Message{
					ID:      "fetch-composed-ws",
					Type:    api.TypeRequest,
					Sender:  m.client.Name(),
					Target:  "project",
					Method:  "project:get-composed-workspace",
					Payload: []byte(event.Data.ContextID),
				}
				m.client.Send(context.Background(), kMsg.Target, kMsg.Method, kMsg.Payload)
			}()

			cmds = append(cmds, m.doDiscovery)
		}
	}

	if displayMsg == "" && msg.Method == "project:get-composed-workspace-resp" {
		var resp struct {
			Workspace  frontend.Workspace `json:"workspace"`
			UserConfig json.RawMessage    `json:"user_config"`
			ActiveID   string             `json:"active_id"`
		}
		if err := json.Unmarshal(msg.Payload, &resp); err == nil {
			// Apply layout from workspace
			if resp.Workspace.Layout != "" {
				var wCfg frontend.WorkspaceConfig
				if err := json.Unmarshal([]byte(resp.Workspace.Layout), &wCfg); err == nil {
					// Apply paney layout logic (duplicated from workspace:opened for now)
					if wCfg.Root != nil {
						m.RootLayout = wCfg.Root
						panes := tui.GetPanes(m.RootLayout)
						if len(panes) > 0 {
							m.FocusedPaneID = panes[0].ID
							m.syncModeWithFocusedPane()
						}
					}
				}
			}
			displayMsg = fmt.Sprintf("[%s] Composed workspace loaded: %s", time.Now().Format("15:04:05"), resp.Workspace.Name)
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
		case "system:trace":
			var trace struct {
				Topic string `json:"topic"`
				Data  struct {
					ID     string `json:"id"`
					Sender string `json:"sender"`
					Target string `json:"target"`
					Method string `json:"method"`
				} `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &trace); err == nil {
				logEntry := fmt.Sprintf("[%s] %s -> %s:%s (%s)",
					time.Now().Format("15:04:05.000"),
					trace.Data.Sender, trace.Data.Target, trace.Data.Method, trace.Data.ID)
				m.inspectorLogs = append(m.inspectorLogs, logEntry)
				if len(m.inspectorLogs) > 500 {
					m.inspectorLogs = m.inspectorLogs[1:]
				}
				m.inspectorVp.SetContent(strings.Join(m.inspectorLogs, "\n"))
				m.inspectorVp.GotoBottom()
			}
			displayMsg = "skip"
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

					// DYNAMIC VIEWPORT PART: Every widget registered also becomes a PANE
					// if we are in a purely dynamic dashboard mode (no set project layout).
					if m.RootLayout.Type == "pane" && m.RootLayout.Mode == "dashboard" && m.RootLayout.PluginID == "" {
						// Single initial pane transition to split
						m.RootLayout = &frontend.LayoutNode{
							Type:      "split",
							Direction: "horizontal",
							Children: []frontend.LayoutNode{
								{ID: w.ID, Type: "pane", Mode: "dashboard", PluginID: w.ID, Weight: 1.0},
							},
						}
						m.FocusedPaneID = w.ID
					} else {
						// Already a split or specific pane, append to first split if it exists
						if m.RootLayout.Type == "split" {
							m.RootLayout.Children = append(m.RootLayout.Children, frontend.LayoutNode{
								ID:       w.ID,
								Type:     "pane",
								Mode:     "dashboard",
								PluginID: w.ID,
								Weight:   1.0,
							})
							// Re-balance weights
							wPct := 1.0 / float64(len(m.RootLayout.Children))
							for i := range m.RootLayout.Children {
								m.RootLayout.Children[i].Weight = wPct
							}
						}
					}
					m.syncModeWithFocusedPane()
				}
				displayMsg = fmt.Sprintf("[%s] Dashboard widget active: %s (%s)", time.Now().Format("15:04:05"), w.Title, msg.Sender)
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
	case "x":
		m.Mode = tui.ModeInspector
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
		panes := tui.GetPanes(m.RootLayout)
		if len(panes) > 1 {
			for i, p := range panes {
				if p.ID == m.FocusedPaneID {
					nextIdx := (i + 1) % len(panes)
					m.FocusedPaneID = panes[nextIdx].ID
					m.syncModeWithFocusedPane()
					break
				}
			}
		}
		return m, nil
	case "ctrl+p":
		m.lastMainMode = m.Mode
		m.Mode = tui.ModeOmni
		m.isLeader = false
		m.commandInput.SetValue("")
		m.commandInput.Focus()
		m.omniResults = nil
		m.omniSelectedIdx = 0
		return m, m.doOmniSearch("")
	case "shift+tab":
		panes := tui.GetPanes(m.RootLayout)
		if len(panes) > 1 {
			for i, p := range panes {
				if p.ID == m.FocusedPaneID {
					prevIdx := (i - 1 + len(panes)) % len(panes)
					m.FocusedPaneID = panes[prevIdx].ID
					m.syncModeWithFocusedPane()
					break
				}
			}
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) syncModeWithFocusedPane() {
	node := tui.FindNodeByID(m.RootLayout, m.FocusedPaneID)
	if node != nil {
		switch node.Mode {
		case "dashboard":
			m.Mode = tui.ModeDashboard
		case "chat":
			m.Mode = tui.ModeChat
		case "editor":
			m.Mode = tui.ModeEdit
		case "inspector":
			m.Mode = tui.ModeInspector
		case "insert":
			m.Mode = tui.ModeInsert
		default:
			if node.PluginID != "" {
				m.Mode = tui.ModeDashboard // Default for plugin panes
			} else {
				m.Mode = tui.ModeNormal
			}
		}
	}
}

func (m *Model) splitFocusedPane(direction string) {
	// Find focused pane node
	foundNode := tui.FindNodeByID(m.RootLayout, m.FocusedPaneID)
	if foundNode == nil {
		return
	}

	// Create a new unique ID for the new pane
	newID := fmt.Sprintf("pane-%d", time.Now().UnixNano())

	// Transform this pane into a split or replace it in its parent
	// For simplicity, we wrap this pane in a new split
	oldNode := *foundNode
	foundNode.Type = "split"
	foundNode.Direction = direction
	foundNode.PluginID = ""
	foundNode.Mode = ""
	foundNode.ID = ""
	foundNode.Children = []frontend.LayoutNode{
		oldNode,
		{
			ID:     newID,
			Type:   "pane",
			Mode:   oldNode.Mode,
			Weight: 0.5,
		},
	}
	// Re-adjust weight for the original node
	foundNode.Children[0].Weight = 0.5

	m.FocusedPaneID = newID
	m.syncModeWithFocusedPane()
}

func (m *Model) closeFocusedPane() {
	// Recursive function to remove node and potentially collapse parent
	var removeNode func(*frontend.LayoutNode) bool
	removeNode = func(node *frontend.LayoutNode) bool {
		if node.Type != "split" {
			return false
		}
		for i, child := range node.Children {
			if child.ID == m.FocusedPaneID {
				node.Children = append(node.Children[:i], node.Children[i+1:]...)
				return true
			}
			if removeNode(&child) {
				// If child became empty, remove it? 
				// For now just check if we need to simplify this split
				if len(child.Children) == 0 && child.Type == "split" {
					node.Children = append(node.Children[:i], node.Children[i+1:]...)
				}
				return true
			}
		}
		return false
	}

	if m.RootLayout.ID == m.FocusedPaneID {
		// Can't close root if only one pane
		return
	}

	removeNode(m.RootLayout)

	// If root split has only one child, collapse it
	if m.RootLayout.Type == "split" && len(m.RootLayout.Children) == 1 {
		*m.RootLayout = m.RootLayout.Children[0]
	}

	// Re-focus something
	panes := tui.GetPanes(m.RootLayout)
	if len(panes) > 0 {
		m.FocusedPaneID = panes[0].ID
		m.syncModeWithFocusedPane()
	}
}

func (m *Model) navigateFocus(dir string) {
	panes := tui.GetPanes(m.RootLayout)
	if len(panes) <= 1 {
		return
	}

	idx := -1
	for i, p := range panes {
		if p.ID == m.FocusedPaneID {
			idx = i
			break
		}
	}

	if idx == -1 {
		m.FocusedPaneID = panes[0].ID
		m.syncModeWithFocusedPane()
		return
	}

	switch dir {
	case "left", "up":
		idx = (idx - 1 + len(panes)) % len(panes)
	case "right", "down":
		idx = (idx + 1) % len(panes)
	}

	m.FocusedPaneID = panes[idx].ID
	m.syncModeWithFocusedPane()
}

func (m Model) sendLayoutUpdate() tea.Cmd {
	if m.RootLayout == nil {
		return nil
	}

	payload, _ := json.Marshal(m.RootLayout)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = m.client.Send(ctx, "project", "update-layout", payload)
		return nil
	}
}

func (m Model) sendViewStateUpdate() tea.Cmd {
	if m.RootLayout == nil {
		return nil
	}

	state := map[string]interface{}{
		"focused_pane_id": m.FocusedPaneID,
		"last_mode":       m.Mode,
		"active_channel":  m.ActiveChannel,
		"active_buffer":   m.activeBuffer,
	}

	payload, _ := json.Marshal(state)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = m.client.Send(ctx, "project", "update-view-state", payload)
		return nil
	}
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
							m.formParams = node.Params
							m.formValues = make([]string, len(node.Params))
							m.formIdx = 0
							m.formError = ""
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
					m.formParams = opt.Params
					m.formValues = make([]string, len(opt.Params))
					m.formIdx = 0
					m.formError = ""
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

func (m Model) validateFormValue(index int, val string) string {
	param := m.formParams[index]
	if param.Required && val == "" {
		return "This field is required"
	}

	switch param.Type {
	case "int":
		var i int
		_, err := fmt.Sscanf(val, "%d", &i)
		if err != nil {
			return "Must be an integer"
		}
	case "bool":
		low := strings.ToLower(val)
		if low != "true" && low != "false" && low != "1" && low != "0" && low != "y" && low != "n" {
			return "Must be true/false, 1/0, or y/n"
		}
	case "enum":
		found := false
		for _, c := range param.Choices {
			if val == c {
				found = true
				break
			}
		}
		if !found {
			return "Must be one of: " + strings.Join(param.Choices, ", ")
		}
	}
	return ""
}

func (m Model) handleFormMode(msg tea.KeyMsg, ciCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlG:
		m.Mode = tui.ModeNormal
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		m.formError = ""
		return m, nil
	case tea.KeyEnter:
		val := m.commandInput.Value()
		errStr := m.validateFormValue(m.formIdx, val)
		if errStr != "" {
			m.formError = errStr
			return m, nil
		}

		m.formValues[m.formIdx] = val
		m.formIdx++
		m.formError = ""
		if m.formIdx >= len(m.formParams) {
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

func (m Model) handleOmniMode(msg tea.KeyMsg, ciCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlG:
		m.Mode = m.lastMainMode
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		m.omniResults = nil
		m.omniList.SetItems(nil)
		return m, nil
	case tea.KeyEnter:
		if sel := m.omniList.SelectedItem(); sel != nil {
			return m.executeOmniResult(sel.(listItem).res)
		}
	case tea.KeyBackspace:
		if m.commandInput.Value() == "" {
			m.Mode = m.lastMainMode
			m.commandInput.Blur()
			return m, nil
		}
	}

	// Update list for navigation/scrolling
	var listCmd tea.Cmd
	m.omniList, listCmd = m.omniList.Update(msg)

	old := m.commandInput.Value()
	m.commandInput, ciCmd = m.commandInput.Update(msg)
	if m.commandInput.Value() != old {
		newVal := m.commandInput.Value()
		// Search debouncing
		return m, tea.Batch(ciCmd, listCmd, func() tea.Msg {
			time.Sleep(150 * time.Millisecond)
			return searchDebounceMsg{Query: newVal}
		})
	}

	return m, tea.Batch(ciCmd, listCmd)
}

func (m Model) doOmniSearch(query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel()

		req := map[string]interface{}{
			"query":     query,
			"limit":     10,
			"buffer_id": m.activeBuffer,
		}
		reqPayload, _ := json.Marshal(req)

		resp, err := m.client.Send(ctx, "omni-palette", "omni:search", reqPayload)
		if err != nil {
			return nil // Silent fail for live search
		}

		var results []OmniResult
		if err := json.Unmarshal(resp.Payload, &results); err != nil {
			return nil
		}
		return results
	}
}

func (m Model) executeOmniResult(res OmniResult) (tea.Model, tea.Cmd) {
	m.Mode = m.lastMainMode
	m.commandInput.Blur()
	m.commandInput.SetValue("")
	m.omniResults = nil
	m.omniList.SetItems(nil)

	action := res.Metadata["action"]
	switch action {
	case "execute":
		return m.executeCommand(res.ID)
	case "switch":
		m.activeBuffer = res.Metadata["buffer_id"]
		m.Mode = tui.ModeEdit
		return m, m.fetchBufferContent(m.activeBuffer)
	case "switch-context":
		return m.executeCommand(fmt.Sprintf("switcher switch %s", res.Metadata["id"]))
	case "open":
		// Assume opening a document means loading it into a buffer
		// We can use buffer:load-file if it exists, or project:open-file
		return m.executeCommand(fmt.Sprintf("buffer load %s", res.Metadata["path"]))
	}
	return m, nil
}
