package main

import (
	"context"
	"flag"
	"fmt"
	"os"
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
)

type model struct {
	client    *frontend.Client
	messages  []string
	viewport  viewport.Model
	textarea  textarea.Model
	discovery []string
	err       error
	width     int
	height    int
	ready     bool
	msgCh     chan api.Message

	// Modal interface state
	mode         int
	commandInput textarea.Model
	activeBuffer string
	isLeader     bool
}

type messageMsg api.Message
type errMsg error
type discoveryMsg string
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
			tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
			m.doDiscovery,
		)

	case discoveryMsg:
		lines := strings.Split(string(msg), "\n")
		var valid []string
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				valid = append(valid, l)
			}
		}
		m.discovery = valid
		return m, nil

	case tea.KeyMsg:
		// Top-level key handling based on mode
		switch m.mode {
		case ModeNormal:
			return m.handleNormalMode(msg)
		case ModeInsert:
			return m.handleInsertMode(msg, tiCmd)
		case ModeCommand:
			return m.handleCommandMode(msg, ciCmd)
		}

	case messageMsg:
		m.messages = append(m.messages, fmt.Sprintf("[%s] %s: %s",
			time.Now().Format("15:04:05"), msg.Sender, string(msg.Payload)))
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

func (m model) handleCommandMode(msg tea.KeyMsg, ciCmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = ModeNormal
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		return m, nil
	case tea.KeyEnter:
		cmd := m.commandInput.Value()
		m.mode = ModeNormal
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		return m.executeCommand(cmd)
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
	return discoveryMsg(string(resp.Payload))
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
		statusStyle.Width(m.width-len(modeStr)).Render(fmt.Sprintf(" Buffer: %s", m.activeBuffer)),
	)

	var mainView string
	if m.mode == ModeInsert {
		mainView = m.textarea.View()
	} else {
		mainView = m.viewport.View()
	}

	view := lipgloss.JoinVertical(lipgloss.Left,
		mainView,
		statusLine,
	)

	if m.mode == ModeCommand {
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
		client:       client,
		textarea:     ta,
		commandInput: ci,
		msgCh:        msgCh,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
