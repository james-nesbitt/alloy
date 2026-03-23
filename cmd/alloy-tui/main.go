package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/cmdutil"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

// Model Layout components

type Model struct {
	client   *frontend.Client
	messages []string
	viewport viewport.Model
	textarea textarea.Model
	targets  []api.Registration
	err      error
	width    int
	height   int
	ready    bool
	msgCh    chan api.Message

	// Modal interface state
	Mode            int
	commandInput    textarea.Model
	activeBuffer    string
	ActiveChannel   string
	isLeader        bool
	breadcrumbs     []string
	subscriptions   map[string]bool
	commandTree     *frontend.CommandNode
	recency         map[string]int
	frequency       map[string]int
	statuses        map[string]string
	selectedCmdIdx  int
	leaderMenuWidth int

	ActiveProject *frontend.Project
	Projects      []frontend.Project
	Workspaces    []frontend.Workspace
	selectType    int

	// Form state
	formTitle  string
	formFields []string
	formValues []string
	formIdx    int

	// Dashboard state
	DashboardTiles map[string]frontend.DashboardTile
	TileOrder      []string

	lastMainMode       int
	localBufferVersion int
	isLocalBufferDirty bool
	remoteCursors      map[string]Cursor

	// Multi-pane state
	Panes []Pane
	FocusIdx int
}

type Pane struct {
	Type int // ModeNormal, ModeDashboard, ModeChat, ModeEdit
	WidthPct float64 // 0.0 to 1.0
}

const (
	SelectNone = iota
	SelectProject
	SelectWorkspace
)

const (
	ModeNormal    = 0
	ModeInsert    = 1
	ModeCommand   = 2
	ModeChat      = 3
	ModeForm      = 4
	ModeDashboard = 5
	ModeEdit      = 6
)

type Cursor struct {
	Row      int    `json:"row"`
	Col      int    `json:"col"`
	User     string `json:"user"`
	LastSeen int64  `json:"last_seen"`
}

type discoveryMsg struct {
	Targets []api.Registration `json:"targets"`
}

type messageMsg api.Message
type errMsg error
type tickMsg time.Time

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.listenForMessages(),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m Model) listenForMessages() tea.Cmd {
	return func() tea.Msg {
		msg := <-m.msgCh
		return messageMsg(msg)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 3
		m.textarea.SetWidth(msg.Width)
		m.commandInput.SetWidth(msg.Width)
		if !m.ready {
			m.ready = true
		}
		return m, nil

	case tickMsg:
		cmds = append(cmds,
			tea.Tick(time.Minute, func(t time.Time) tea.Msg { return tickMsg(t) }),
			m.doDiscovery,
			m.sendPresenceHeartbeat(),
		)

	case tea.KeyMsg:
		// Global quit
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}

		// Pane switching
		if msg.String() == "ctrl+w" {
			// This is just a modifier prefix in many layouts, but let's make it a simple toggle for now
			// Or wait for next key...
		}

		// Handle keys based on mode
		switch m.Mode {
		case ModeNormal, ModeDashboard:
			newM, cmd := m.handleNormalMode(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			m = newM.(Model)
			var vpCmd tea.Cmd
			m.viewport, vpCmd = m.viewport.Update(msg)
			if vpCmd != nil {
				cmds = append(cmds, vpCmd)
			}

		case ModeInsert:
			newM, cmd := m.handleInsertMode(msg, nil)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			m = newM.(Model)
			var taCmd tea.Cmd
			m.textarea, taCmd = m.textarea.Update(msg)
			if taCmd != nil {
				cmds = append(cmds, taCmd)
			}

		case ModeChat:
			newM, cmd := m.handleChatMode(msg, nil)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			m = newM.(Model)
			var taCmd tea.Cmd
			m.textarea, taCmd = m.textarea.Update(msg)
			if taCmd != nil {
				cmds = append(cmds, taCmd)
			}

		case ModeEdit:
			newM, cmd := m.handleEditMode(msg, nil)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			m = newM.(Model)

			oldVal := m.textarea.Value()
			oldLine := m.textarea.Line()
			var taCmd tea.Cmd
			m.textarea, taCmd = m.textarea.Update(msg)
			if taCmd != nil {
				cmds = append(cmds, taCmd)
			}
			if m.textarea.Value() != oldVal {
				m.isLocalBufferDirty = true
			}
			if m.textarea.Line() != oldLine {
				cmds = append(cmds, m.sendCursorUpdate(m.activeBuffer, m.textarea.Line(), 0))
			}

		case ModeCommand:
			newM, cmd := m.handleCommandMode(msg, nil)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			m = newM.(Model)

		case ModeForm:
			newM, cmd := m.handleFormMode(msg, nil)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			m = newM.(Model)
		}

	case discoveryMsg:
		m.targets = msg.Targets
		if m.statuses == nil {
			m.statuses = make(map[string]string)
		}
		for _, t := range m.targets {
			if t.Status != "" {
				m.statuses[t.ID] = t.Status
			}
		}
		m.commandTree = frontend.BuildCommandTree(m.targets)
		for _, t := range m.targets {
			if t.ID == "events" && !m.subscriptions["chat:message"] {
				cmds = append(cmds, m.subscribe("chat:message"), m.subscribe("chat:direct"),
					m.subscribe("chat:presence"), m.subscribe("project:opened"),
					m.subscribe("workspace:opened"),
					m.subscribe("presence:online"), m.subscribe("presence:offline"),
					m.subscribe("presence:heartbeat"),
					m.subscribe("plugin:crashed"), m.subscribe("plugin:load_failed"),
					m.subscribe("buffer:update"), m.subscribe("buffer:cursors_updated"),
					m.subscribe("dashboard:widget-registered"),
					m.subscribe("dashboard:widget-updated"),
					m.subscribe("dashboard:widget-unregistered"),
					m.subscribe("component:registered"))
				m.subscriptions["chat:message"] = true
				m.subscriptions["chat:direct"] = true
				m.subscriptions["chat:presence"] = true
				m.subscriptions["project:opened"] = true
				m.subscriptions["workspace:opened"] = true
				m.subscriptions["presence:online"] = true
				m.subscriptions["presence:offline"] = true
				m.subscriptions["presence:heartbeat"] = true
				m.subscriptions["component:registered"] = true

				// NEW: Request initial dashboard widgets
				cmds = append(cmds, m.fetchInitialWidgets())
			}
			if t.ID == "project" && m.ActiveProject == nil {
				cmds = append(cmds, m.fetchActiveProject())
			}
		}

	case messageMsg:
		m2 := &m
		cmds = append(cmds, m2.processMessage(api.Message(msg)))
		m = *m2

	case errMsg:
		m.messages = append(m.messages, lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("!! "+msg.Error()))
		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()
	}

	// Sync mode with focused pane
	if len(m.Panes) > 0 && m.FocusIdx < len(m.Panes) {
		m.Panes[m.FocusIdx].Type = m.Mode
	}

	return m, tea.Batch(cmds...)
}

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
				newPanes := []Pane{}
				for _, lp := range m.ActiveProject.Layout.Layout {
					p := Pane{WidthPct: lp.WidthPct}
					switch lp.Type {
					case "dashboard":
						p.Type = ModeDashboard
					case "chat":
						p.Type = ModeChat
					case "editor":
						p.Type = ModeEdit
					default:
						p.Type = ModeNormal
					}
					newPanes = append(newPanes, p)
				}
				m.Panes = newPanes
				m.FocusIdx = 0
				m.Mode = m.Panes[0].Type
			} else {
				if m.ActiveProject.Layout.DefaultMode == "dashboard" {
					m.Mode = ModeDashboard
				} else if m.ActiveProject.Layout.DefaultMode == "chat" {
					m.Mode = ModeChat
				}
				m.Panes = []Pane{{Type: m.Mode, WidthPct: 1.0}}
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
						newPanes := []Pane{}
						for _, lp := range wCfg.Layout {
							p := Pane{WidthPct: lp.WidthPct}
							switch lp.Type {
							case "dashboard":
								p.Type = ModeDashboard
							case "chat":
								p.Type = ModeChat
							case "editor":
								p.Type = ModeEdit
							default:
								p.Type = ModeNormal
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
			ID      string            `json:"id"`
			Content []byte            `json:"content"`
			Version int               `json:"version"`
			Cursors map[string]Cursor `json:"cursors,omitempty"`
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

func (m Model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case ":", "alt+x":
		m.lastMainMode = m.Mode
		m.Mode = ModeCommand
		m.isLeader = false
		m.commandInput.SetValue(":")
		m.commandInput.Focus()
		return m, nil
	case " ":
		m.lastMainMode = m.Mode
		m.Mode = ModeCommand
		m.isLeader = true
		m.commandInput.SetValue("")
		m.commandInput.Focus()
		return m, nil
	case "i":
		m.Mode = ModeInsert
		m.textarea.Focus()
		return m, nil
	case "d":
		m.Mode = ModeDashboard
		m.isLeader = false
		return m, nil
	case "v":
		m.Mode = ModeNormal
		m.isLeader = false
		return m, nil
	case "c":
		m.Mode = ModeChat
		m.textarea.Placeholder = "Type message to #" + m.ActiveChannel + "..."
		m.textarea.Focus()
		return m, nil
	case "ctrl+c", "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleInsertMode(msg tea.KeyMsg, tiCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.Mode = ModeNormal
		m.textarea.Blur()
		return m, nil
	}
	return m, tiCmd
}

func (m Model) handleChatMode(msg tea.KeyMsg, tiCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.Mode = ModeNormal
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
		m.Mode = ModeNormal
		m.textarea.Blur()
		return m, nil
	case tea.KeyCtrlS:
		// Manual save
		return m, m.sendBufferUpdate(m.activeBuffer, m.textarea.Value(), false)
	}

	// For real-time sync, we could check if value changed and send update debounced.
	// But let's start with explicit save or simple auto-save.

	return m, tiCmd
}

func (m Model) fetchBufferContent(id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		payload, _ := json.Marshal(map[string]string{"id": id})
		resp, err := m.client.Send(ctx, "buffer", "read", payload)
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
		resp, err := m.client.Send(ctx, "buffer", "write", payload)
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
		_, _ = m.client.Send(ctx, "buffer", "update_cursor", payload)
		return nil
	}
}

func (m Model) subscribe(topic string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		payload, _ := json.Marshal(map[string]string{"topic": topic})
		_, _ = m.client.Send(ctx, "events", "subscribe", payload)
		return nil
	}
}

func (m Model) sendChatMessage(content string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		var method string
		var payload []byte

		if strings.HasPrefix(m.ActiveChannel, "dm:") {
			method = "direct:send"
			payload, _ = json.Marshal(map[string]string{
				"to":      m.ActiveChannel[3:],
				"content": content,
			})
		} else {
			method = "send"
			payload, _ = json.Marshal(map[string]string{
				"channel": m.ActiveChannel,
				"content": content,
			})
		}

		_, err := m.client.Send(ctx, "chat", method, payload)
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

		// Legacy chat update
		payload, _ := json.Marshal(map[string]string{
			"status": "online",
		})
		_, _ = m.client.Send(ctx, "chat", "presence:update", payload)

		// New team-presence event
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
		_, _ = m.client.Send(ctx, "events", "publish", eventData)

		return nil
	}
}

func (m Model) fetchActiveProject() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, _ := m.client.Send(ctx, "project", "active", nil)
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
		resp, _ := m.client.Send(ctx, "project", "list", nil)
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
		resp, _ := m.client.Send(ctx, "project", "list-workspaces", nil)
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

func (m Model) handleCommandMode(msg tea.KeyMsg, ciCmd tea.Cmd) (tea.Model, tea.Cmd) {
	// 1. Pre-update Check: Drill-down shortcuts for Leader Mode
	// We check this BEFORE updating the input so that keys like 'p' can be
	// captured as drill-down actions instead of text input.
	if m.isLeader && msg.Type == tea.KeyRunes && m.commandInput.Value() == "" {
		char := string(msg.Runes)
		if char == ":" {
			m.isLeader = false
			m.breadcrumbs = nil
			m.commandInput.SetValue(":")
			m.selectedCmdIdx = 0
			return m, nil
		}

		// If typing the exact shortcut key of a child, drill down immediately
		node := m.commandTree.Find(m.breadcrumbs)
		if node != nil {
			if child, ok := node.Children[char]; ok {
				if len(child.Children) == 0 {
					m.Mode = m.lastMainMode
					if m.Mode == ModeCommand {
						m.Mode = ModeNormal
					} // Failsafe
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
					// Success! Swallowed the key and drilled down.
					return m, nil
				}
			}
		}
	}

	// 2. Normal Input Handling
	// First, let the input handle the key unless it's navigation
	switch msg.Type {
	case tea.KeyDown, tea.KeyCtrlN, tea.KeyUp, tea.KeyCtrlP, tea.KeyEnter, tea.KeyEsc, tea.KeyCtrlG:
		// Navigation/Termination handled below
	default:
		m.commandInput, ciCmd = m.commandInput.Update(msg)
		m.selectedCmdIdx = 0
	}

	filteredCount := len(m.filteredCommands())

	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlG:
		m.Mode = m.lastMainMode
		if m.Mode == ModeCommand {
			m.Mode = ModeNormal
		} // Failsafe
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		m.breadcrumbs = nil
		m.selectedCmdIdx = 0
		m.isLeader = false // Ensure leader is reset
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
		if filteredCount > 0 && m.selectedCmdIdx >= 0 && m.selectedCmdIdx < len(filtered) {
			opt := filtered[m.selectedCmdIdx]
			if m.selectType == SelectProject {
				m.Mode = m.lastMainMode
				if m.Mode == ModeCommand {
					m.Mode = ModeNormal
				} // Failsafe
				m.commandInput.Blur()
				m.selectType = SelectNone
				m.commandInput.SetValue("")
				m.isLeader = false // Reset leader
				return m.executeCommand(fmt.Sprintf("project open %s", opt.Raw))
			}
			if m.selectType == SelectWorkspace {
				m.Mode = m.lastMainMode
				if m.Mode == ModeCommand {
					m.Mode = ModeNormal
				} // Failsafe
				m.commandInput.Blur()
				m.selectType = SelectNone
				m.commandInput.SetValue("")
				m.isLeader = false // Reset leader
				return m.executeCommand(fmt.Sprintf("project set-workspace %s", opt.Raw))
			}
			if m.isLeader {
				if opt.IsDir {
					m.breadcrumbs = append(m.breadcrumbs, opt.Display)
					m.selectedCmdIdx = 0
					m.commandInput.SetValue("")
					return m, nil
				} else {
					// Find the node to execute it properly
					node := m.commandTree.Find(append(m.breadcrumbs, opt.Display))
					if node != nil {
						m.Mode = m.lastMainMode
						if m.Mode == ModeCommand {
							m.Mode = ModeNormal
						} // Failsafe
						m.commandInput.Blur()
						m.commandInput.SetValue("")
						m.breadcrumbs = nil
						m.selectedCmdIdx = 0
						m.isLeader = false // Explicitly reset before executing
						return m.executeCommand(fmt.Sprintf("%s %s", node.Target, node.Method))
					}
				}
			} else {
				m.Mode = m.lastMainMode
				if m.Mode == ModeCommand {
					m.Mode = ModeNormal
				} // Failsafe
				m.commandInput.Blur()
				m.commandInput.SetValue("")
				m.breadcrumbs = nil
				m.selectedCmdIdx = 0
				m.isLeader = false
				return m.executeCommand(opt.Raw)
			}
		}

		cmd := m.commandInput.Value()
		m.Mode = m.lastMainMode
		if m.Mode == ModeCommand {
			m.Mode = ModeNormal
		} // Failsafe
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		m.breadcrumbs = nil
		m.selectedCmdIdx = 0
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
		m.Mode = ModeNormal
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		return m, nil
	case tea.KeyEnter:
		m.formValues[m.formIdx] = m.commandInput.Value()
		m.formIdx++
		if m.formIdx >= len(m.formFields) {
			m.Mode = ModeNormal
			m.commandInput.Blur()
			m.commandInput.SetValue("")

			switch m.formTitle {
			case "Create New Project":
				name := m.formValues[0]
				desc := m.formValues[1]
				payload, _ := json.Marshal(map[string]string{
					"name":        name,
					"description": desc,
				})
				return m, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					resp, _ := m.client.Send(ctx, "project", "create", payload)
					return messageMsg(resp)
				}

			case "Switch AI Provider":
				t := m.formValues[0]
				modelVal := m.formValues[1]
				url := m.formValues[2]
				payload, _ := json.Marshal(map[string]string{
					"type":  t,
					"model": modelVal,
					"url":   url,
				})
				return m, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					resp, _ := m.client.Send(ctx, "ai", "provider:set", payload)
					return messageMsg(resp)
				}

			case "AI Query":
				prompt := m.formValues[0]
				payload, _ := json.Marshal(map[string]string{
					"prompt": prompt,
				})
				return m, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					resp, _ := m.client.Send(ctx, "ai", "query", payload)
					return messageMsg(resp)
				}
			}
			return m, nil
		}
		m.commandInput.SetValue("")
		m.commandInput.Placeholder = m.formFields[m.formIdx] + "..."
		return m, nil
	}
	m.commandInput, ciCmd = m.commandInput.Update(msg)
	return m, ciCmd
}

func (m Model) doDiscovery() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := m.client.Send(ctx, "command-manager", "discover", nil)
	if err != nil {
		return nil
	}
	var dMsg discoveryMsg
	if err := json.Unmarshal(resp.Payload, &dMsg); err != nil {
		return nil
	}
	return dMsg
}

func (m Model) executeCommand(cmdStr string) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return m, nil
	}

	// Handle traditional ':' or leader commands
	if strings.HasPrefix(cmdStr, ":") {
		cmdStr = cmdStr[1:]
	}

	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return m, nil
	}

	// Reset state for new command
	m.isLeader = false
	m.selectType = SelectNone
	m.breadcrumbs = nil

	verb := parts[0]
	switch verb {
	case "ai":
		if len(parts) >= 2 && (parts[1] == "switch" || parts[1] == "provider:set") {
			m.Mode = ModeForm
			m.formTitle = "Switch AI Provider"
			m.formFields = []string{"Type (ollama|openai|anthropic)", "Model", "URL (optional)"}
			m.formValues = make([]string, 3)
			m.formIdx = 0
			m.commandInput.SetValue("")
			m.commandInput.Placeholder = "Provider Type..."
			m.commandInput.Focus()
			return m, nil
		}
		if len(parts) >= 2 && parts[1] == "query" {
			if len(parts) == 2 {
				m.Mode = ModeForm
				m.formTitle = "AI Query"
				m.formFields = []string{"Prompt"}
				m.formValues = make([]string, 1)
				m.formIdx = 0
				m.commandInput.SetValue("")
				m.commandInput.Placeholder = "Ask the AI..."
				m.commandInput.Focus()
				return m, nil
			}
		}
	case "p", "project":
		if len(parts) >= 2 && parts[1] == "open" {
			if len(parts) == 2 {
				m.Mode = ModeCommand
				m.selectType = SelectProject
				m.commandInput.Focus()
				m.commandInput.SetValue("")
				return m, m.fetchProjects()
			}
			// if len(parts) > 2, it falls through to the default plugin call
		} else if len(parts) >= 2 && parts[1] == "list-workspaces" {
			m.Mode = ModeCommand
			m.selectType = SelectWorkspace
			m.commandInput.Focus()
			m.commandInput.SetValue("")
			return m, m.fetchWorkspaces()
		} else if len(parts) >= 2 && parts[1] == "create" {
			if len(parts) == 2 {
				m.Mode = ModeForm
				m.formTitle = "Create New Project"
				m.formFields = []string{"Name", "Description"}
				m.formValues = make([]string, 2)
				m.formIdx = 0
				m.commandInput.SetValue("")
				m.commandInput.Placeholder = "Project Name..."
				m.commandInput.Focus()
				return m, nil
			}
		}
	case "q", "quit":
		return m, tea.Quit
	case "b", "buffer":
		if len(parts) > 1 {
			m.activeBuffer = parts[1]
			cmds = append(cmds, m.fetchBufferContent(m.activeBuffer))
		}
	case "e", "edit":
		if len(parts) > 1 {
			m.activeBuffer = parts[1]
			m.Mode = ModeEdit
			m.textarea.Focus()
			cmds = append(cmds, m.fetchBufferContent(m.activeBuffer))
		}
	case "join":
		if len(parts) > 1 {
			m.ActiveChannel = parts[1]
		}
	case "dm":
		if len(parts) > 1 {
			target := parts[1]
			// The plugin expects dm:A:B where A < B
			m.ActiveChannel = "dm:" + target
		}
	case "ls":
		// List logic...
	case "vsplit":
		if len(m.Panes) < 3 {
			m.Panes = append(m.Panes, Pane{Type: ModeChat, WidthPct: 0.5})
			// Adjust widths
			for i := range m.Panes {
				m.Panes[i].WidthPct = 1.0 / float64(len(m.Panes))
			}
			m.FocusIdx = len(m.Panes) - 1
			m.Mode = m.Panes[m.FocusIdx].Type
		}
	case "focus-next":
		m.FocusIdx = (m.FocusIdx + 1) % len(m.Panes)
		m.Mode = m.Panes[m.FocusIdx].Type
	case "close-pane":
		if len(m.Panes) > 1 {
			m.Panes = append(m.Panes[:m.FocusIdx], m.Panes[m.FocusIdx+1:]...)
			m.FocusIdx = 0
			for i := range m.Panes {
				m.Panes[i].WidthPct = 1.0 / float64(len(m.Panes))
			}
			m.Mode = m.Panes[m.FocusIdx].Type
		}
	default:
		// Attempt to call plugin
		if len(parts) >= 2 {
			target := parts[0]
			method := parts[1]
			payload := ""

			// Increment recency & frequency for "plugin-id method"
			if m.recency == nil {
				m.recency = make(map[string]int)
			}
			if m.frequency == nil {
				m.frequency = make(map[string]int)
			}
			key := target + " " + method
			m.recency[key] = int(time.Now().Unix())
			m.frequency[key]++

			if len(parts) > 2 {
				payload = strings.Join(parts[2:], " ")
			}
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				resp, err := m.client.Send(ctx, target, method, []byte(payload))
				if err != nil {
					return errMsg(err)
				}
				return messageMsg(resp)
			}
		}
	}
	return m, tea.Batch(cmds...)
}

type CommandOption struct {
	Raw         string
	Display     string
	Description string
	Annotation  string
	IsDir       bool
	Method      string
	Status      string
	Frequency   int
	Score       int
}

func (m Model) filteredCommands() []CommandOption {
	var results []CommandOption

	if m.Mode == ModeCommand && m.selectType == SelectProject {
		input := m.commandInput.Value()
		for _, p := range m.Projects {
			score := frontend.FuzzyScore(p.Name, input)
			if score > 0 {
				results = append(results, CommandOption{
					Raw:         p.ID,
					Display:     p.Name,
					Description: p.Description,
					Score:       score,
				})
			}
		}
	} else if m.Mode == ModeCommand && m.selectType == SelectWorkspace {
		input := m.commandInput.Value()
		for _, w := range m.Workspaces {
			score := frontend.FuzzyScore(w.Name, input)
			if score > 0 {
				results = append(results, CommandOption{
					Raw:         w.ID,
					Display:     w.Name,
					Description: w.Path,
					Score:       score,
				})
			}
		}
	} else if m.Mode == ModeCommand && !m.isLeader {
		input := m.commandInput.Value()
		if len(input) > 0 && input[0] == ':' {
			input = input[1:]
		}

		if m.commandTree == nil {
			return nil
		}

		// Flatten the entire tree and fuzzy find
		flattened := m.commandTree.Flatten("")
		for _, item := range flattened {
			score := frontend.FuzzyScore(item.FullTitle, input)
			if score > 0 {
				status := "running"
				if s, ok := m.statuses[item.Target]; ok {
					status = s
				}

				results = append(results, CommandOption{
					Raw:         item.Target + " " + item.Method,
					Display:     item.FullTitle,
					Description: item.Description,
					Status:      status,
					Frequency:   m.frequency[item.Target+" "+item.Method],
					Score:       score,
				})
			}
		}

		// Weight by recency/frequency/status
		sort.Slice(results, func(i, j int) bool {
			// 1. Status priority
			if results[i].Status != results[j].Status {
				if results[i].Status == "crashed" {
					return false
				}
				if results[j].Status == "crashed" {
					return true
				}
			}

			// 2. Fuzzy Score (Prefix match + score)
			if results[i].Score != results[j].Score {
				return results[i].Score > results[j].Score
			}

			// 3. Recency (last used in session)
			ri := m.recency[results[i].Raw]
			rj := m.recency[results[j].Raw]
			if ri != rj {
				return ri > rj
			}

			// 4. Frequency
			fi := results[i].Frequency
			fj := results[j].Frequency
			if fi != fj {
				return fi > fj
			}

			return results[i].Display < results[j].Display
		})
	} else if m.Mode == ModeCommand && m.isLeader && m.commandTree != nil {
		node := m.commandTree.Find(m.breadcrumbs)
		if node != nil {
			input := m.commandInput.Value()
			var keys []string
			for k := range node.Children {
				keys = append(keys, k)
			}

			for _, k := range keys {
				child := node.Children[k]
				// Match against key or method name
				scoreK := frontend.FuzzyScore(k, input)
				scoreM := frontend.FuzzyScore(child.Method, input)
				score := max(scoreK, scoreM)

				if score > 0 {
					status := "running"
					if s, ok := m.statuses[child.Target]; ok {
						status = s
					}

					results = append(results, CommandOption{
						Raw:         child.Target + " " + child.Method,
						Display:     k,
						Description: child.Description,
						Annotation:  child.Annotation,
						IsDir:       len(child.Children) > 0,
						Method:      child.Method,
						Status:      status,
						Frequency:   m.frequency[child.Target+" "+child.Method],
						Score:       score,
					})
				}
			}

			// Weight by recency/frequency/status
			sort.Slice(results, func(i, j int) bool {
				if results[i].Status != results[j].Status {
					if results[i].Status == "crashed" {
						return false
					}
					if results[j].Status == "crashed" {
						return true
					}
				}

				if results[i].Score != results[j].Score {
					return results[i].Score > results[j].Score
				}

				ri := m.recency[results[i].Raw]
				rj := m.recency[results[j].Raw]
				if ri != rj {
					return ri > rj
				}

				return results[i].Display < results[j].Display
			})
		}
	}

	if len(results) > 10 {
		results = results[:10]
	}
	return results
}

func (m Model) leaderMenuView() string {
	if !m.isLeader || m.commandTree == nil {
		return ""
	}

	node := m.commandTree.Find(m.breadcrumbs)
	if node == nil {
		return ""
	}

	keys := make([]string, 0, len(node.Children))
	for k := range node.Children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var items []string
	for _, k := range keys {
		child := node.Children[k]
		label := k
		desc := child.Description
		if len(child.Children) > 0 {
			desc = "..."
		}

		item := lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).Bold(true).Render(label) + " " +
			lipgloss.NewStyle().Foreground(lipgloss.Color("200")).Render("→") + " " +
			lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(desc)

		items = append(items, item)
	}

	// Calculate column layout - simple for now, but better than flat list
	columnCount := 3
	if len(items) < 5 {
		columnCount = 1
	}

	var rows []string
	for i := 0; i < len(items); i += columnCount {
		rowItems := items[i:min(i+columnCount, len(items))]
		row := ""
		for _, ri := range rowItems {
			row += fmt.Sprintf("%-25s", ri) + "  "
		}
		rows = append(rows, row)
	}

	title := " " + strings.Join(append([]string{"Leader"}, m.breadcrumbs...), " > ") + " "
	titleStyle := lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("255")).Bold(true)

	menuBody := strings.Join(rows, "\n")

	menuStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1)

	return "\n" + titleStyle.Render(title) + "\n" + menuStyle.Render(menuBody)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m Model) dashboardView() string {
	if len(m.TileOrder) == 0 {
		return lipgloss.NewStyle().
			Width(m.width).
			Height(m.height-3).
			Align(lipgloss.Center, lipgloss.Center).
			Render("No dynamic dashboard widgets registered.\nWaiting for WASM plugins to initialize...")
	}

	cols := 2
	if m.width < 100 {
		cols = 1
	}

	tileStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Margin(1, 1).
		Width((m.width / cols) - 4).
		Height((m.height / 3) - 2)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var rows []string
	var currentRow []string

	for i, id := range m.TileOrder {
		tile := m.DashboardTiles[id]

		var content string
		if tile.ContentType == "json" {
			// Pretty print JSON
			var obj interface{}
			if err := json.Unmarshal(tile.RawContent, &obj); err == nil {
				pretty, _ := json.MarshalIndent(obj, "", "  ")
				content = string(pretty)
			} else {
				content = string(tile.RawContent)
			}
		} else {
			// Default to text/markdown list
			if len(tile.Content) > 0 && tile.Content[0] != "" {
				content = strings.Join(tile.Content, "\n")
			} else {
				content = string(tile.RawContent)
			}
		}

		footer := ""
		if len(tile.Actions) > 0 {
			footer = "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("Actions: "+strings.Join(tile.Actions, ", "))
		}

		tileView := tileStyle.Render(
			lipgloss.JoinVertical(lipgloss.Left,
				lipgloss.JoinHorizontal(lipgloss.Top, titleStyle.Render(tile.Title), " ", statusStyle.Render(" "+tile.Status)),
				"",
				contentStyle.Render(content),
				footer,
			),
		)

		currentRow = append(currentRow, tileView)
		if (i+1)%cols == 0 || i == len(m.TileOrder)-1 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, currentRow...))
			currentRow = []string{}
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m Model) renderPaneContent(paneType int, width int, height int) string {
	if paneType == ModeInsert || paneType == ModeChat || paneType == ModeEdit {
		m.textarea.SetWidth(width)
		m.textarea.SetHeight(height)
		return m.textarea.View()
	} else if paneType == ModeDashboard {
		// Dashboard needs careful handling of child widget widths
		// Temporarily adjust m.width/height for dashboardView logic
		oldW, oldH := m.width, m.height
		m.width, m.height = width, height+3
		view := m.dashboardView()
		m.width, m.height = oldW, oldH
		return view
	}

	m.viewport.Width = width
	m.viewport.Height = height
	return m.viewport.View()
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	modeStr := " NORMAL "
	modeStyle := lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Bold(true)

	switch m.Mode {
	case ModeInsert:
		modeStr = " INSERT "
		modeStyle = modeStyle.Background(lipgloss.Color("2"))
	case ModeChat:
		modeStr = " CHAT "
		modeStyle = modeStyle.Background(lipgloss.Color("6"))
	case ModeForm:
		modeStr = " FORM "
		modeStyle = modeStyle.Background(lipgloss.Color("13"))
	case ModeDashboard:
		modeStr = " DASHBOARD "
		modeStyle = modeStyle.Background(lipgloss.Color("14"))
	case ModeEdit:
		modeStr = " EDIT "
		modeStyle = modeStyle.Background(lipgloss.Color("2"))
	case ModeCommand:
		if m.isLeader {
			modeStr = " LEADER "
			modeStyle = modeStyle.Background(lipgloss.Color("3"))
		} else {
			modeStr = " COMMAND "
			modeStyle = modeStyle.Background(lipgloss.Color("5"))
		}
	}

	statusStyle := lipgloss.NewStyle().Background(lipgloss.Color("8")).Foreground(lipgloss.Color("15"))
	projectStr := " No Project "
	if m.ActiveProject != nil {
		projectStr = " Project: " + m.ActiveProject.Name + " "
	}

	remoteStr := ""
	if m.remoteCursors != nil {
		for u := range m.remoteCursors {
			if u == m.client.Actor() {
				continue
			}
			remoteStr += " | " + u
		}
	}

	statusLine := lipgloss.JoinHorizontal(lipgloss.Center,
		modeStyle.Render(modeStr),
		statusStyle.Width(m.width-len(modeStr)-len(projectStr)).Render(fmt.Sprintf(" Buffer: %s | Channel: #%s%s", m.activeBuffer, m.ActiveChannel, remoteStr)),
		modeStyle.Background(lipgloss.Color("12")).Render(projectStr),
	)

	var mainView string
	workingHeight := m.height - 3
	if m.Mode == ModeCommand || m.Mode == ModeForm {
		workingHeight = (m.height * 2) / 3
	}

	if len(m.Panes) > 1 {
		var views []string
		for i, p := range m.Panes {
			paneWidth := int(float64(m.width) * p.WidthPct)
			if i == len(m.Panes)-1 {
				// Fill remaining width
				used := 0
				for j := 0; j < i; j++ {
					used += int(float64(m.width) * m.Panes[j].WidthPct)
				}
				paneWidth = m.width - used
			}

			// Add visual border for focused pane
			style := lipgloss.NewStyle().Width(paneWidth).Height(workingHeight)
			if i == m.FocusIdx {
				style = style.Border(lipgloss.DoubleBorder(), false, true, false, true).BorderForeground(lipgloss.Color("62"))
			} else {
				style = style.Border(lipgloss.NormalBorder(), false, true, false, true).BorderForeground(lipgloss.Color("240"))
			}

			content := m.renderPaneContent(p.Type, paneWidth-2, workingHeight)
			views = append(views, style.Render(content))
		}
		mainView = lipgloss.JoinHorizontal(lipgloss.Top, views...)
	} else {
		mainView = m.renderPaneContent(m.Mode, m.width, workingHeight)
	}

	view := lipgloss.JoinVertical(lipgloss.Left,
		mainView,
		statusLine,
	)

	if m.Mode == ModeCommand {
		prompt := ":"
		if m.isLeader {
			prompt = strings.Join(m.breadcrumbs, " > ")
			if prompt != "" {
				prompt += " > "
			}
		}
		m.commandInput.Placeholder = prompt

		// Add Leader Menu or Filtered Commands above the command bar
		if m.isLeader && m.commandInput.Value() == "" {
			view = lipgloss.JoinVertical(lipgloss.Left, view, m.leaderMenuView())
		} else {
			filtered := m.filteredCommands()
			if len(filtered) > 0 {
				// Live Preview Panel
				selected := filtered[m.selectedCmdIdx]

				previewStyle := lipgloss.NewStyle().
					Background(lipgloss.Color("0")).
					Foreground(lipgloss.Color("7")).
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("62")).
					Padding(0, 1).
					Width(m.width - 2)

				// Construct detailed preview
				statusLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("● ONLINE")
				if s, ok := m.statuses[strings.Split(selected.Raw, " ")[0]]; ok && s == "crashed" {
					statusLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("✘ CRASHED")
				}

				previewContent := fmt.Sprintf(
					"%s  %s\n\n%s\n\n%s",
					lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")).Render(selected.Display),
					statusLabel,
					lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(selected.Description),
					lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("Usage: "+selected.Raw),
				)

				view = lipgloss.JoinVertical(lipgloss.Left, view, previewStyle.Render(previewContent))

				listStyle := lipgloss.NewStyle().Background(lipgloss.Color("0")).Foreground(lipgloss.Color("7")).Width(m.width)
				selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Bold(true)
				marginaliaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)

				var rows []string
				lastTarget := ""
				for i, opt := range filtered {
					// Grouping
					target := strings.Split(opt.Raw, " ")[0]
					if !m.isLeader && target != lastTarget {
						groupHeader := lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true).Render(" ── " + strings.ToUpper(target) + " ")
						rows = append(rows, groupHeader)
						lastTarget = target
					}

					label := opt.Display
					statusStr := ""
					switch opt.Status {
					case "crashed":
						statusStr = " (CRASHED)"
					case "error":
						statusStr = " (ERROR)"
					case "loading":
						statusStr = " (LOADING...)"
					case "registered":
						statusStr = " (LAZY)"
					}

					var line string
					if m.isLeader {
						annotation := ""
						if opt.Annotation != "" {
							annotation = fmt.Sprintf("[%s] ", opt.Annotation)
						}
						method := opt.Method
						if opt.IsDir {
							method = "..."
						}

						// Multi-column leader style
						left := fmt.Sprintf(" %-2s %s%-15s", opt.Display, annotation, method)
						right := marginaliaStyle.Render(opt.Description + statusStr)

						// Calculate padding
						pad := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
						if pad < 0 {
							pad = 0
						}
						line = left + strings.Repeat(" ", pad) + right
					} else {
						// standard command style
						left := " " + label
						right := marginaliaStyle.Render(opt.Description + statusStr)

						pad := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
						if pad < 0 {
							pad = 0
						}
						line = left + strings.Repeat(" ", pad) + right
					}

					if i == m.selectedCmdIdx {
						rows = append(rows, selectedStyle.Width(m.width).Render(line))
					} else {
						switch opt.Status {
						case "crashed", "error":
							rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(line))
						case "loading":
							rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(line))
						case "registered":
							rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(line))
						default:
							rows = append(rows, line)
						}
					}
				}

				listStr := "\n" + strings.Join(rows, "\n")
				view = lipgloss.JoinVertical(lipgloss.Left, view, listStyle.Render(listStr))
			}
		}
		view = lipgloss.JoinVertical(lipgloss.Left, view, m.commandInput.View())
	} else if m.Mode == ModeForm {
		formStyle := lipgloss.NewStyle().Background(lipgloss.Color("0")).Foreground(lipgloss.Color("7")).Width(m.width)
		labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))

		var rows []string
		rows = append(rows, lipgloss.NewStyle().Bold(true).Underline(true).Render(m.formTitle))

		for i, field := range m.formFields {
			val := m.formValues[i]
			if i == m.formIdx {
				rows = append(rows, labelStyle.Render("> "+field+": ")+m.commandInput.Value())
			} else if i < m.formIdx {
				rows = append(rows, "  "+field+": "+val)
			} else {
				rows = append(rows, "  "+field+": ")
			}
		}

		listStr := "\n" + strings.Join(rows, "\n")
		view = lipgloss.JoinVertical(lipgloss.Left, view, formStyle.Render(listStr))
		view = lipgloss.JoinVertical(lipgloss.Left, view, m.commandInput.View())
	}

	return view
}

func NewModel(client *frontend.Client, msgCh chan api.Message) Model {
	ta := textarea.New()
	ta.Placeholder = "Write content here..."
	ta.SetHeight(5)

	ci := textarea.New()
	ci.Placeholder = ":"
	ci.SetHeight(1)

	return Model{
		client:        client,
		textarea:      ta,
		commandInput:  ci,
		msgCh:         msgCh,
		ActiveChannel: "general",
		Mode:          ModeDashboard,
		subscriptions: make(map[string]bool),
		recency:       make(map[string]int),
		DashboardTiles: make(map[string]frontend.DashboardTile),
		TileOrder:      nil,
		Panes: []Pane{
			{Type: ModeDashboard, WidthPct: 1.0},
		},
		FocusIdx: 0,
	}
}

func main() {
	name := flag.String("name", "alloy-tui", "Name of the TUI component")
	actor := flag.String("actor", "", "Actor identity (defaults to name)")
	socket := flag.String("socket", frontend.GetAlloyRuntimeDir()+"/default.sock", "Socket address")
	debug := flag.Bool("debug", false, "Enable debug logging")
	sf := cmdutil.RegisterSecurityFlags(flag.CommandLine)
	flag.Parse()

	cmdutil.HandleSecurityError(sf.Validate())

	// Set up logging
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	if *actor == "" {
		*actor = *name
	}

	msgCh := make(chan api.Message, 100)
	client, err := frontend.NewClientWithActorAndSecurity(*name, *actor, *socket, *sf.Insecure, *sf.SecurityDir)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		os.Exit(1)
	}

	client.OnMessage(func(msg api.Message) {
		logger.Debug("received message", "sender", msg.Sender, "method", msg.Method)
		msgCh <- msg
	})

	m := NewModel(client, msgCh)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
