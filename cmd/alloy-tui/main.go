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
const (
	ModeNormal  = 0
	ModeInsert  = 1
	ModeCommand = 2
	ModeChat    = 3
)

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
		m.commandInput, ciCmd = m.commandInput.Update(msg)
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
				m.subscriptions["chat:message"] = true
				m.subscriptions["chat:direct"] = true
				m.subscriptions["chat:presence"] = true
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
		}

	case messageMsg:
		var displayMsg string
		if msg.Sender == "plugin-events" {
			switch msg.Method {
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
	case ":":
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

func (m model) handleCommandMode(msg tea.KeyMsg, ciCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = ModeNormal
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		m.breadcrumbs = nil
		return m, nil
	case tea.KeyEnter:
		cmd := m.commandInput.Value()
		m.mode = ModeNormal
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		m.breadcrumbs = nil
		return m.executeCommand(cmd)
	case tea.KeyBackspace:
		if m.commandInput.Value() == "" && len(m.breadcrumbs) > 0 {
			m.breadcrumbs = m.breadcrumbs[:len(m.breadcrumbs)-1]
		}
	}

	// Dynamic sequence handling for Leader mode
	if m.isLeader && msg.Type == tea.KeyRunes {
		char := string(msg.Runes)
		m.breadcrumbs = append(m.breadcrumbs, char)

		// Check for shortcut matches in the tree
		node := m.commandTree.Find(m.breadcrumbs)
		if node != nil {
			if len(node.Children) == 0 {
				// We found a leaf match! Execute it.
				m.mode = ModeNormal
				m.commandInput.Blur()
				m.commandInput.SetValue("")
				m.breadcrumbs = nil
				return m.executeCommand(fmt.Sprintf("%s %s", node.Target, node.Method))
			}
			// Otherwise just stay in leader mode and wait for the rest of the sequence
		} else {
			// No match, clear breadcrumbs or just stay?
			// Emacs style: keep waiting but show it's invalid
			m.breadcrumbs = m.breadcrumbs[:len(m.breadcrumbs)-1]
		}
	}

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

func (m model) filteredCommands() []string {
	if m.mode == ModeCommand && !m.isLeader {
		input := m.commandInput.Value()
		if len(input) > 0 && input[0] == ':' {
			input = input[1:]
		}
		
		var matched []string
		for _, t := range m.targets {
			for _, c := range t.Capabilities {
				full := t.ID + " " + c.Method
				if strings.Contains(full, input) || input == "" {
					matched = append(matched, fmt.Sprintf(" %-20s %s", full, c.Description))
				}
			}
		}
		if len(matched) > 10 {
			matched = matched[:10]
		}
		return matched
	}

	if !m.isLeader || m.commandTree == nil {
		return nil
	}

	node := m.commandTree.Find(m.breadcrumbs)
	if node == nil {
		return nil
	}

	var matched []string
	keys := make([]string, 0, len(node.Children))
	for k := range node.Children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		child := node.Children[k]
		annotation := ""
		if child.Annotation != "" {
			annotation = fmt.Sprintf("[%s] ", child.Annotation)
		}

		label := child.Method
		if len(child.Children) > 0 {
			label = "..."
		}

		matched = append(matched, fmt.Sprintf(" %-2s  %s%-15s %s",
			k, annotation, label, child.Description))
	}
	return matched
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
	statusLine := lipgloss.JoinHorizontal(lipgloss.Center,
		modeStyle.Render(modeStr),
		statusStyle.Width(m.width-len(modeStr)).Render(fmt.Sprintf(" Buffer: %s | Channel: #%s", m.activeBuffer, m.activeChannel)),
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

			// Add filtered commands above the command bar
			filtered := m.filteredCommands()
			if len(filtered) > 0 {
				listStyle := lipgloss.NewStyle().Background(lipgloss.Color("0")).Foreground(lipgloss.Color("7")).Width(m.width)
				listStr := "\n" + strings.Join(filtered, "\n")
				view = lipgloss.JoinVertical(lipgloss.Left, view, listStyle.Render(listStr))
			}
		}
		m.commandInput.Placeholder = prompt
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
