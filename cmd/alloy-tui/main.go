package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/frontend"
)

// TUI Layout components

type model struct {
	client    *frontend.Client
	messages  []string
	viewport  viewport.Model
	textarea  textarea.Model
	targets   []registration
	err       error
	width     int
	height    int
	ready     bool
	msgCh     chan api.Message

	// Modal interface state
	mode          int
	commandInput  textarea.Model
	activeBuffer  string
	activeChannel string
	isLeader      bool
	breadcrumbs   []string
	subscriptions map[string]bool
	commandTree   *CommandNode
	selectedCmdIdx int

	activeProject *Project
	projects      []Project
	selectType    int

	// Form state
	formTitle  string
	formFields []string
	formValues []string
	formIdx    int
}

const (
	SelectNone = iota
	SelectProject
)

const (
	ModeNormal  = 0
	ModeInsert  = 1
	ModeCommand = 2
	ModeChat    = 3
	ModeForm    = 4
)

type Project struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
}

type registration struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Capabilities []api.Capability `json:"capabilities,omitempty"`
}

type discoveryMsg struct {
	Targets []registration `json:"targets"`
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
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
		ciCmd tea.Cmd
	)

	// In Command mode, only update the command input.
	if m.mode == ModeCommand {
		// skip manual update here as it's handled in handleCommandMode specifically
	} else if m.mode == ModeInsert {
		m.textarea, tiCmd = m.textarea.Update(msg)
	}

	m.viewport, vpCmd = m.viewport.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 3 // Adjust for status and command line
		m.textarea.SetWidth(msg.Width)
		m.commandInput.SetWidth(msg.Width)

		if !m.ready {
			m.ready = true
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(
			tea.Tick(time.Minute, func(t time.Time) tea.Msg { return tickMsg(t) }),
			m.doDiscovery,
			m.sendPresenceHeartbeat(),
		)

	case discoveryMsg:
		m.targets = msg.Targets
		m.commandTree = BuildCommandTree(m.targets)
		var cmds []tea.Cmd
		for _, t := range m.targets {
			if t.ID == "plugin-events" && !m.subscriptions["chat:message"] {
				cmds = append(cmds, m.subscribe("chat:message"))
				cmds = append(cmds, m.subscribe("chat:direct"))
				cmds = append(cmds, m.subscribe("chat:presence"))
				cmds = append(cmds, m.subscribe("project:opened"))
				m.subscriptions["chat:message"] = true
				m.subscriptions["chat:direct"] = true
				m.subscriptions["chat:presence"] = true
				m.subscriptions["project:opened"] = true
				cmds = append(cmds, m.subscribe("plugin:crashed"))
				cmds = append(cmds, m.subscribe("plugin:load_failed"))
			}
			if t.ID == "plugin-project-manager" && m.activeProject == nil {
				cmds = append(cmds, m.fetchActiveProject())
			}
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		// Top-level key handling based on mode
		switch m.mode {
		case ModeNormal:
			return m.handleNormalMode(msg)
		case ModeInsert:
			return m.handleInsertMode(msg, tiCmd)
		case ModeChat:
			return m.handleChatMode(msg, tiCmd)
		case ModeCommand:
			return m.handleCommandMode(msg, ciCmd)
		case ModeForm:
			return m.handleFormMode(msg, ciCmd)
		}

	case messageMsg:
		var displayMsg string
		if msg.Sender == "plugin-events" && msg.Method == "project:opened" {
			var event struct {
				Topic string  `json:"topic"`
				Data  Project `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &event); err == nil {
				m.activeProject = &event.Data
				displayMsg = fmt.Sprintf("[%s] Project opened: %s", time.Now().Format("15:04:05"), m.activeProject.Name)
			}
		}

		if displayMsg == "" && msg.Sender == "plugin-project-manager" && msg.Method == "active-resp" {
			var p Project
			if err := json.Unmarshal(msg.Payload, &p); err == nil {
				m.activeProject = &p
			}
		}

		if displayMsg == "" && msg.Sender == "plugin-project-manager" && msg.Method == "list-resp" {
			var resp struct {
				Projects []Project `json:"projects"`
			}
			if err := json.Unmarshal(msg.Payload, &resp); err == nil {
				m.projects = resp.Projects
				// If we were waiting for projects, don't display the raw JSON
				return m, m.listenForMessages()
			}
		}

		if displayMsg == "" && msg.Sender == "plugin-events" {
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
			case "chat:direct":
				var dm struct {
					From    string `json:"from"`
					To      string `json:"to"`
					Content string `json:"content"`
				}
				if err := json.Unmarshal(msg.Payload, &dm); err == nil {
					displayMsg = fmt.Sprintf("[%s] DM (%s > %s) %s",
						time.Now().Format("15:04:05"), dm.From, dm.To, dm.Content)
				}
			case "chat:presence":
				var pres struct {
					User   string `json:"user"`
					Status string `json:"status"`
				}
				if err := json.Unmarshal(msg.Payload, &pres); err == nil {
					displayMsg = fmt.Sprintf("[%s] * %s is now %s",
						time.Now().Format("15:04:05"), pres.User, pres.Status)
				}
			}
		}

		if displayMsg == "" {
			if msg.Type == api.TypeResponse {
				displayMsg = fmt.Sprintf("[%s] < %s", time.Now().Format("15:04:05"), string(msg.Payload))
			} else {
				displayMsg = fmt.Sprintf("[%s] %s: %s",
					time.Now().Format("15:04:05"), msg.Sender, string(msg.Payload))
			}
		}

		m.messages = append(m.messages, displayMsg)
		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()
		return m, m.listenForMessages()

	case errMsg:
		m.messages = append(m.messages, lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("!! "+msg.Error()))
		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()
		return m, nil
	}

	return m, tea.Batch(tiCmd, vpCmd, ciCmd)
}

func (m model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case ":", "alt+x":
		m.mode = ModeCommand
		m.isLeader = false
		m.commandInput.SetValue(":")
		m.commandInput.Focus()
		return m, nil
	case " ":
		m.mode = ModeCommand
		m.isLeader = true
		m.commandInput.SetValue("")
		m.commandInput.Focus()
		return m, nil
	case "i":
		m.mode = ModeInsert
		m.textarea.Focus()
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
		_, _ = m.client.Send(ctx, "plugin-events", "subscribe", payload)
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

		_, err := m.client.Send(ctx, "plugin-chat", method, payload)
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
		payload, _ := json.Marshal(map[string]string{
			"status": "online",
		})
		_, _ = m.client.Send(ctx, "plugin-chat", "presence:update", payload)
		return nil
	}
}

func (m model) fetchActiveProject() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, _ := m.client.Send(ctx, "plugin-project-manager", "active", nil)
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
		resp, _ := m.client.Send(ctx, "plugin-project-manager", "list", nil)
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
		m.mode = ModeNormal
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
				m.mode = ModeNormal
				m.commandInput.Blur()
				m.selectType = SelectNone
				m.commandInput.SetValue("")
				return m.executeCommand(fmt.Sprintf("plugin-project-manager open %s", opt.Raw))
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
						m.mode = ModeNormal
						m.commandInput.Blur()
						m.commandInput.SetValue("")
						m.breadcrumbs = nil
						m.selectedCmdIdx = 0
						return m.executeCommand(fmt.Sprintf("%s %s", node.Target, node.Method))
					}
				}
			} else {
				m.mode = ModeNormal
				m.commandInput.Blur()
				m.commandInput.SetValue("")
				m.breadcrumbs = nil
				m.selectedCmdIdx = 0
				return m.executeCommand(opt.Raw)
			}
		}

		cmd := m.commandInput.Value()
		m.mode = ModeNormal
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		m.breadcrumbs = nil
		m.selectedCmdIdx = 0
		return m.executeCommand(cmd)
	case tea.KeyBackspace:
		if m.commandInput.Value() == "" && len(m.breadcrumbs) > 0 {
			m.breadcrumbs = m.breadcrumbs[:len(m.breadcrumbs)-1]
		}
	}

	if m.isLeader && msg.Type == tea.KeyRunes {
		char := string(msg.Runes)
		// If typing the exact shortcut key of a child, drill down immediately
		node := m.commandTree.Find(m.breadcrumbs)
		if node != nil {
			if child, ok := node.Children[char]; ok {
				m.breadcrumbs = append(m.breadcrumbs, char)
				m.commandInput.SetValue("")
				if len(child.Children) == 0 {
					m.mode = ModeNormal
					m.commandInput.Blur()
					m.breadcrumbs = nil
					m.selectedCmdIdx = 0
					return m.executeCommand(fmt.Sprintf("%s %s", child.Target, child.Method))
				}
				return m, nil
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
					resp, _ := m.client.Send(ctx, "plugin-project-manager", "create", payload)
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
					resp, _ := m.client.Send(ctx, "plugin-ai-agent", "provider:set", payload)
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
					resp, _ := m.client.Send(ctx, "plugin-ai-agent", "query", payload)
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
	resp, err := m.client.Send(ctx, "plugin-command-manager", "discover", nil)
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
	case "ai", "plugin-ai-agent":
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
	case "p", "plugin-project-manager":
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

func fuzzyMatch(target, input string) bool {
	if input == "" {
		return true
	}
	target = strings.ToLower(target)
	input = strings.ToLower(input)

	inputIdx := 0
	for targetIdx := 0; targetIdx < len(target); targetIdx++ {
		if target[targetIdx] == input[inputIdx] {
			inputIdx++
		}
		if inputIdx == len(input) {
			return true
		}
	}
	return false
}

type CommandOption struct {
	Raw         string
	Display     string
	Description string
	Annotation  string
	IsDir       bool
	Method      string
}

func (m model) filteredCommands() []CommandOption {
	var results []CommandOption

	if m.mode == ModeCommand && m.selectType == SelectProject {
		input := m.commandInput.Value()
		for _, p := range m.projects {
			if fuzzyMatch(p.Name, input) {
				results = append(results, CommandOption{
					Raw:         p.ID,
					Display:     p.Name,
					Description: p.Description,
				})
			}
		}
	} else if m.mode == ModeCommand && !m.isLeader {
		input := m.commandInput.Value()
		if len(input) > 0 && input[0] == ':' {
			input = input[1:]
		}
		
		for _, t := range m.targets {
			for _, c := range t.Capabilities {
				full := t.ID + " " + c.Method
				if fuzzyMatch(full, input) {
					results = append(results, CommandOption{
						Raw:         full,
						Display:     full,
						Description: c.Description,
					})
				}
			}
		}
	} else if m.mode == ModeCommand && m.isLeader && m.commandTree != nil {
		node := m.commandTree.Find(m.breadcrumbs)
		if node != nil {
			input := m.commandInput.Value()
			keys := make([]string, 0, len(node.Children))
			for k := range node.Children {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, k := range keys {
				child := node.Children[k]
				
				// Label to match against can be the key (k) or the Method 
				matchStr := k + " " + child.Method
				if fuzzyMatch(matchStr, input) {
					results = append(results, CommandOption{
						Raw:         child.Key,
						Display:     k,
						Description: child.Description,
						Annotation:  child.Annotation,
						IsDir:       len(child.Children) > 0,
						Method:      child.Method,
					})
				}
			}
		}
	}

	if len(results) > 10 {
		results = results[:10]
	}
	return results
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
	if m.mode == ModeInsert || m.mode == ModeChat {
		mainView = m.textarea.View()
	} else {
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

		// Add filtered commands above the command bar
		filtered := m.filteredCommands()
		if len(filtered) > 0 {
			// Live Preview Panel
			selected := filtered[m.selectedCmdIdx]
			previewStyle := lipgloss.NewStyle().Background(lipgloss.Color("8")).Foreground(lipgloss.Color("7")).Padding(0, 1).Width(m.width)
			previewView := previewStyle.Render(fmt.Sprintf("\n PREVIEW: %s\n Description: %s\n", selected.Display, selected.Description))
			view = lipgloss.JoinVertical(lipgloss.Left, view, previewView)

			listStyle := lipgloss.NewStyle().Background(lipgloss.Color("0")).Foreground(lipgloss.Color("7")).Width(m.width)
			selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Bold(true)

			var rows []string
			for i, opt := range filtered {
				label := opt.Display
				if m.isLeader {
					annotation := ""
					if opt.Annotation != "" {
						annotation = fmt.Sprintf("[%s] ", opt.Annotation)
					}
					method := opt.Method
					if opt.IsDir {
						method = "..."
					}
					label = fmt.Sprintf("%-2s  %s%-15s", opt.Display, annotation, method)
				}

				line := fmt.Sprintf(" %-20s %s", label, opt.Description)
				if i == m.selectedCmdIdx {
					rows = append(rows, selectedStyle.Render(line))
				} else {
					rows = append(rows, line)
				}
			}

			listStr := "\n" + strings.Join(rows, "\n")
			view = lipgloss.JoinVertical(lipgloss.Left, view, listStyle.Render(listStr))
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
	socket := flag.String("socket", frontend.GetAlloyRuntimeDir()+"/default.sock", "Socket address")
	insecure := flag.Bool("insecure", false, "Disable mTLS")
	flag.Parse()

	msgCh := make(chan api.Message, 100)
	client, err := frontend.NewClient(*name, *socket, *insecure)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		os.Exit(1)
	}

	client.OnMessage(func(msg api.Message) {
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
		subscriptions: make(map[string]bool),
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
