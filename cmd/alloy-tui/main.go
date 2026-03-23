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

// TUI Layout components

type model struct {
	client   *frontend.Client
	messages []string
	viewport viewport.Model
	textarea textarea.Model
	targets  []frontend.Registration
	err      error
	width    int
	height   int
	ready    bool
	msgCh    chan api.Message

	// Modal interface state
	mode            int
	commandInput    textarea.Model
	activeBuffer    string
	activeChannel   string
	isLeader        bool
	breadcrumbs     []string
	subscriptions   map[string]bool
	commandTree     *frontend.CommandNode
	recency         map[string]int
	frequency       map[string]int
	statuses        map[string]string
	selectedCmdIdx  int
	leaderMenuWidth int

	activeProject *Project
	projects      []Project
	selectType    int

	// Form state
	formTitle  string
	formFields []string
	formValues []string
	formIdx    int

	// Dashboard state
	dashboardTiles map[string]DashboardTile
	tileOrder      []string

	lastMainMode int
}

const (
	SelectNone = iota
	SelectProject
)

const (
	ModeNormal    = 0
	ModeInsert    = 1
	ModeCommand   = 2
	ModeChat      = 3
	ModeForm      = 4
	ModeDashboard = 5
)

type DashboardTile struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Content   []string `json:"content"`
	Status    string   `json:"status"`
	Actions   []string `json:"actions"`
	Timestamp int64    `json:"timestamp"`
}

type WorkspaceConfig struct {
	DefaultMode string `json:"default_mode"`
	Dashboard   struct {
		Tiles []struct {
			Plugin string `json:"plugin"`
			Weight int    `json:"weight"`
		} `json:"tiles"`
	} `json:"dashboard"`
}

type Project struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Layout      WorkspaceConfig `json:"layout,omitempty"`
}

type Presence struct {
	User      string `json:"user"`
	Status    string `json:"status"`
	LastSeen  int64  `json:"last_seen"`
	Client    string `json:"client"`
	ProjectID string `json:"project_id,omitempty"`
}

type discoveryMsg struct {
	Targets []frontend.Registration `json:"targets"`
}

type messageMsg api.Message
type errMsg error
type tickMsg time.Time

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.listenForMessages(),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m model) listenForMessages() tea.Cmd {
	return func() tea.Msg {
		msg := <-m.msgCh
		return messageMsg(msg)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

		// Handle keys based on mode
		switch m.mode {
		case ModeNormal, ModeDashboard:
			newM, cmd := m.handleNormalMode(msg)
			if cmd != nil { cmds = append(cmds, cmd) }
			m = newM.(model)
			var vpCmd tea.Cmd
			m.viewport, vpCmd = m.viewport.Update(msg)
			if vpCmd != nil { cmds = append(cmds, vpCmd) }

		case ModeInsert:
			newM, cmd := m.handleInsertMode(msg, nil)
			if cmd != nil { cmds = append(cmds, cmd) }
			m = newM.(model)
			var taCmd tea.Cmd
			m.textarea, taCmd = m.textarea.Update(msg)
			if taCmd != nil { cmds = append(cmds, taCmd) }

		case ModeChat:
			newM, cmd := m.handleChatMode(msg, nil)
			if cmd != nil { cmds = append(cmds, cmd) }
			m = newM.(model)
			var taCmd tea.Cmd
			m.textarea, taCmd = m.textarea.Update(msg)
			if taCmd != nil { cmds = append(cmds, taCmd) }

		case ModeCommand:
			newM, cmd := m.handleCommandMode(msg, nil)
			if cmd != nil { cmds = append(cmds, cmd) }
			m = newM.(model)

		case ModeForm:
			newM, cmd := m.handleFormMode(msg, nil)
			if cmd != nil { cmds = append(cmds, cmd) }
			m = newM.(model)
		}

	case discoveryMsg:
		m.targets = msg.Targets
		if m.statuses == nil { m.statuses = make(map[string]string) }
		for _, t := range m.targets {
			if t.Status != "" { m.statuses[t.ID] = t.Status }
		}
		m.commandTree = frontend.BuildCommandTree(m.targets)
		for _, t := range m.targets {
			if t.ID == "events" && !m.subscriptions["chat:message"] {
				cmds = append(cmds, m.subscribe("chat:message"), m.subscribe("chat:direct"), 
					m.subscribe("chat:presence"), m.subscribe("project:opened"), 
					m.subscribe("plugin:crashed"), m.subscribe("plugin:load_failed"))
				m.subscriptions["chat:message"] = true
				m.subscriptions["chat:direct"] = true
				m.subscriptions["chat:presence"] = true
				m.subscriptions["project:opened"] = true
			}
			if t.ID == "project" && m.activeProject == nil {
				cmds = append(cmds, m.fetchActiveProject())
			}
		}

	case messageMsg:
		cmds = append(cmds, m.processMessage(api.Message(msg)))

	case errMsg:
		m.messages = append(m.messages, lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("!! "+msg.Error()))
		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()
	}

	return m, tea.Batch(cmds...)
}

func (m model) processMessage(msg api.Message) tea.Cmd {
	var displayMsg string
	if msg.Sender == "events" && msg.Method == "project:opened" {
		var event struct {
			Topic string  `json:"topic"`
			Data  Project `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &event); err == nil {
			m.activeProject = &event.Data
			displayMsg = fmt.Sprintf("[%s] Project opened: %s", time.Now().Format("15:04:05"), m.activeProject.Name)
			if m.activeProject.Layout.DefaultMode == "dashboard" {
				m.mode = ModeDashboard
			} else if m.activeProject.Layout.DefaultMode == "chat" {
				m.mode = ModeChat
			}
		}
	}

	if msg.Method == "dashboard-update" {
		var tile DashboardTile
		if err := json.Unmarshal(msg.Payload, &tile); err == nil {
			if m.dashboardTiles == nil { m.dashboardTiles = make(map[string]DashboardTile) }
			m.dashboardTiles[msg.Sender] = tile
			found := false
			for _, id := range m.tileOrder {
				if id == msg.Sender { found = true; break }
			}
			if !found { m.tileOrder = append(m.tileOrder, msg.Sender) }
		}
		return m.listenForMessages()
	}

	if displayMsg == "" && msg.Sender == "project" && msg.Method == "active-resp" {
		var p Project
		if err := json.Unmarshal(msg.Payload, &p); err == nil {
			m.activeProject = &p
		}
	}

	if displayMsg == "" && msg.Sender == "project" && msg.Method == "list-resp" {
		var resp struct {
			Projects []Project `json:"projects"`
		}
		if err := json.Unmarshal(msg.Payload, &resp); err == nil {
			m.projects = resp.Projects
			return m.listenForMessages()
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
				if m.statuses == nil { m.statuses = make(map[string]string) }
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
		}
	}

	if displayMsg == "" {
		if msg.Type == api.TypeResponse {
			displayMsg = fmt.Sprintf("[%s] < %s", time.Now().Format("15:04:05"), string(msg.Payload))
		} else {
			displayMsg = fmt.Sprintf("[%s] %s: %s", time.Now().Format("15:04:05"), msg.Sender, string(msg.Payload))
		}
	}

	m.messages = append(m.messages, displayMsg)
	m.viewport.SetContent(strings.Join(m.messages, "\n"))
	m.viewport.GotoBottom()
	return m.listenForMessages()
}

func (m model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case ":", "alt+x":
		m.lastMainMode = m.mode
		m.mode = ModeCommand
		m.isLeader = false
		m.commandInput.SetValue(":")
		m.commandInput.Focus()
		return m, nil
	case " ":
		m.lastMainMode = m.mode
		m.mode = ModeCommand
		m.isLeader = true
		m.commandInput.SetValue("")
		m.commandInput.Focus()
		return m, nil
	case "i":
		m.mode = ModeInsert
		m.textarea.Focus()
		return m, nil
	case "d":
		m.mode = ModeDashboard
		m.isLeader = false
		return m, nil
	case "v":
		m.mode = ModeNormal
		m.isLeader = false
		return m, nil
	case "c":
		m.mode = ModeChat
		m.textarea.Placeholder = "Type message to #" + m.activeChannel + "..."
		m.textarea.Focus()
		return m, nil
	case "ctrl+c", "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m model) handleInsertMode(msg tea.KeyMsg, tiCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.mode = ModeNormal
		m.textarea.Blur()
		return m, nil
	}
	return m, tiCmd
}

func (m model) handleChatMode(msg tea.KeyMsg, tiCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = ModeNormal
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

func (m model) subscribe(topic string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		payload, _ := json.Marshal(map[string]string{"topic": topic})
		_, _ = m.client.Send(ctx, "events", "subscribe", payload)
		return nil
	}
}

func (m model) sendChatMessage(content string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		var method string
		var payload []byte

		if strings.HasPrefix(m.activeChannel, "dm:") {
			method = "direct:send"
			payload, _ = json.Marshal(map[string]string{
				"to":      m.activeChannel[3:],
				"content": content,
			})
		} else {
			method = "send"
			payload, _ = json.Marshal(map[string]string{
				"channel": m.activeChannel,
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

func (m model) sendPresenceHeartbeat() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		
		// Legacy chat update
		payload, _ := json.Marshal(map[string]string{
			"status": "online",
		})
		_, _ = m.client.Send(ctx, "chat", "presence:update", payload)

		// New team-presence event
		presence := Presence{
			User:    m.client.Actor(),
			Status:  "online",
			Client:  "tui",
			LastSeen: time.Now().Unix(),
		}
		if m.activeProject != nil {
			presence.ProjectID = m.activeProject.ID
		}
		
		eventData, _ := json.Marshal(map[string]any{
			"topic": "presence:heartbeat",
			"data":  presence,
		})
		_, _ = m.client.Send(ctx, "events", "publish", eventData)

		return nil
	}
}

func (m model) fetchActiveProject() tea.Cmd {
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

func (m model) fetchProjects() tea.Cmd {
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

func (m model) handleCommandMode(msg tea.KeyMsg, ciCmd tea.Cmd) (tea.Model, tea.Cmd) {
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
		m.mode = m.lastMainMode
		if m.mode == ModeCommand { m.mode = ModeNormal } // Failsafe
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		m.breadcrumbs = nil
		m.selectedCmdIdx = 0
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
				m.mode = m.lastMainMode
				if m.mode == ModeCommand { m.mode = ModeNormal } // Failsafe
				m.commandInput.Blur()
				m.selectType = SelectNone
				m.commandInput.SetValue("")
				return m.executeCommand(fmt.Sprintf("project open %s", opt.Raw))
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
						m.mode = m.lastMainMode
						if m.mode == ModeCommand { m.mode = ModeNormal } // Failsafe
						m.commandInput.Blur()
						m.commandInput.SetValue("")
						m.breadcrumbs = nil
						m.selectedCmdIdx = 0
						return m.executeCommand(fmt.Sprintf("%s %s", node.Target, node.Method))
					}
				}
			} else {
				m.mode = m.lastMainMode
				if m.mode == ModeCommand { m.mode = ModeNormal } // Failsafe
				m.commandInput.Blur()
				m.commandInput.SetValue("")
				m.breadcrumbs = nil
				m.selectedCmdIdx = 0
				return m.executeCommand(opt.Raw)
			}
		}

		cmd := m.commandInput.Value()
		m.mode = m.lastMainMode
		if m.mode == ModeCommand { m.mode = ModeNormal } // Failsafe
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		m.breadcrumbs = nil
		m.selectedCmdIdx = 0
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
					m.mode = m.lastMainMode
					if m.mode == ModeCommand { m.mode = ModeNormal } // Failsafe
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

	return m, ciCmd
}

func (m model) handleFormMode(msg tea.KeyMsg, ciCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlG:
		m.mode = ModeNormal
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		return m, nil
	case tea.KeyEnter:
		m.formValues[m.formIdx] = m.commandInput.Value()
		m.formIdx++
		if m.formIdx >= len(m.formFields) {
			m.mode = ModeNormal
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
				model := m.formValues[1]
				url := m.formValues[2]
				payload, _ := json.Marshal(map[string]string{
					"type":  t,
					"model": model,
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

func (m model) doDiscovery() tea.Msg {
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

func (m model) executeCommand(cmdStr string) (tea.Model, tea.Cmd) {
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

	verb := parts[0]
	switch verb {
	case "ai":
		if len(parts) >= 2 && (parts[1] == "switch" || parts[1] == "provider:set") {
			m.mode = ModeForm
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
				m.mode = ModeForm
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
				m.mode = ModeCommand
				m.selectType = SelectProject
				m.commandInput.Focus()
				m.commandInput.SetValue("")
				return m, m.fetchProjects()
			}
			// if len(parts) > 2, it falls through to the default plugin call
		} else if len(parts) >= 2 && parts[1] == "create" {
			if len(parts) == 2 {
				m.mode = ModeForm
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
		}
	case "join":
		if len(parts) > 1 {
			m.activeChannel = parts[1]
		}
	case "dm":
		if len(parts) > 1 {
			target := parts[1]
			// The plugin expects dm:A:B where A < B
			m.activeChannel = "dm:" + target
		}
	case "ls":
		// List logic...
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
	return m, nil
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

func (m model) filteredCommands() []CommandOption {
	var results []CommandOption

	if m.mode == ModeCommand && m.selectType == SelectProject {
		input := m.commandInput.Value()
		for _, p := range m.projects {
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
	} else if m.mode == ModeCommand && !m.isLeader {
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
	} else if m.mode == ModeCommand && m.isLeader && m.commandTree != nil {
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

func (m model) leaderMenuView() string {
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

func (m model) dashboardView() string {
	if len(m.tileOrder) == 0 {
		return lipgloss.NewStyle().
			Width(m.width).
			Height(m.height - 3).
			Align(lipgloss.Center, lipgloss.Center).
			Render("No active dashboard tiles.\nPress ':' and type 'dashboard' to see providers.")
	}

	tileStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Margin(1, 1).
		Width((m.width / 2) - 4).
		Height((m.height / 3) - 2)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var rows []string
	var currentRow []string

	for i, id := range m.tileOrder {
		tile := m.dashboardTiles[id]
		
		content := strings.Join(tile.Content, "\n")
		footer := ""
		if len(tile.Actions) > 0 {
			footer = "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("Actions: " + strings.Join(tile.Actions, ", "))
		}

		tileView := tileStyle.Render(
			lipgloss.JoinVertical(lipgloss.Left,
				lipgloss.JoinHorizontal(lipgloss.Top, titleStyle.Render(tile.Title), " ", statusStyle.Render(" "+tile.Status)),
				"",
				content,
				footer,
			),
		)

		currentRow = append(currentRow, tileView)
		if (i+1)%2 == 0 || i == len(m.tileOrder)-1 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, currentRow...))
			currentRow = []string{}
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	modeStr := " NORMAL "
	modeStyle := lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Bold(true)

	switch m.mode {
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
	if m.activeProject != nil {
		projectStr = " Project: " + m.activeProject.Name + " "
	}
	statusLine := lipgloss.JoinHorizontal(lipgloss.Center,
		modeStyle.Render(modeStr),
		statusStyle.Width(m.width-len(modeStr)-len(projectStr)).Render(fmt.Sprintf(" Buffer: %s | Channel: #%s", m.activeBuffer, m.activeChannel)),
		modeStyle.Background(lipgloss.Color("12")).Render(projectStr),
	)

	var mainView string
	renderMode := m.mode
	workingHeight := m.height - 3
	if m.mode == ModeCommand || m.mode == ModeForm {
		renderMode = m.lastMainMode
		workingHeight = (m.height * 2) / 3
	}

	if renderMode == ModeInsert || renderMode == ModeChat {
		m.textarea.SetHeight(workingHeight)
		mainView = m.textarea.View()
	} else if renderMode == ModeDashboard {
		// Temporarily adjust m.height for dashboardView logic
		oldHeight := m.height
		m.height = workingHeight + 3
		mainView = m.dashboardView()
		m.height = oldHeight
	} else {
		m.viewport.Height = workingHeight
		mainView = m.viewport.View()
	}

	view := lipgloss.JoinVertical(lipgloss.Left,
		mainView,
		statusLine,
	)

	if m.mode == ModeCommand {
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
	} else if m.mode == ModeForm {
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

	ta := textarea.New()
	ta.Placeholder = "Write content here..."
	ta.SetHeight(5)

	ci := textarea.New()
	ci.Placeholder = ":"
	ci.SetHeight(1)

	m := model{
		client:        client,
		textarea:      ta,
		commandInput:  ci,
		msgCh:         msgCh,
		activeChannel: "general",
		mode:          ModeDashboard,
		subscriptions: make(map[string]bool),
		recency:       make(map[string]int),
		dashboardTiles: map[string]DashboardTile{
			"team": {
				Title:   "Team Presence",
				Content: []string{"● You (Online)", "○ James (Away)", "● AI Worker (Idle)"},
				Status:  "Active",
				Actions: []string{"Invite", "Call"},
			},
			"ai": {
				Title:   "AI Assistant",
				Content: []string{"● Ollama (Running)", "○ Active Model: llama3", "Tasks: 0/1 completed"},
				Actions: []string{"Query", "Summarize"},
			},
			"chat": {
				Title:   "Team Chat",
				Content: []string{"#general", "<James> Anyone online?", "<AI> Ready to help."},
				Status:  "2 unread",
				Actions: []string{"Open", "Clear"},
			},
			"project": {
				Title:   "Current Project",
				Content: []string{"Phase: 5 (Team Collaboration)", "Branch: feature/phase-5-uipolish", "Health: Stable"},
				Status:  "OK",
			},
		},
		tileOrder: []string{"ai", "chat", "project"},
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
