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

type model struct {
	client     *frontend.Client
	messages   []string
	viewport   viewport.Model
	textarea   textarea.Model
	discovery  []string
	err        error
	width      int
	height     int
	ready      bool
	msgCh      chan api.Message
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
	)

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	switch msg := msg.(type) {
	case tickMsg:
		return m, tea.Batch(
			tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
			func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				resp, err := m.client.Send(ctx, "plugin-command-manager", "discover", nil)
				if err != nil {
					return nil
				}
				return discoveryMsg(string(resp.Payload))
			},
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
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			content := m.textarea.Value()
			if content == "" {
				return m, nil
			}

			parts := strings.SplitN(strings.TrimSpace(content), " ", 3)
			if len(parts) >= 2 {
				target := parts[0]
				method := parts[1]
				payload := ""
				if len(parts) == 3 {
					payload = parts[2]
				}

				m.messages = append(m.messages, lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Render("-> "+content))
				m.textarea.Reset()
				m.viewport.SetContent(strings.Join(m.messages, "\n"))
				m.viewport.GotoBottom()

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

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-6)
			m.viewport.SetContent("Connected to Alloy Core.")
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 6
		}
		m.textarea.SetWidth(msg.Width)

	case messageMsg:
		m.messages = append(m.messages, lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(fmt.Sprintf("<- [%s] %s", msg.Sender, string(msg.Payload))))
		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()
		return m, m.listenForMessages()

	case errMsg:
		m.messages = append(m.messages, lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("!! "+msg.Error()))
		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()
	}

	return m, tea.Batch(tiCmd, vpCmd)
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("3")).
		Render("Alloy TUI | Available Plugins:")
	
	discoveryText := strings.Join(m.discovery, "  ")
	if len(discoveryText) > m.width {
		discoveryText = discoveryText[:m.width-3] + "..."
	}
	discoveryView := lipgloss.NewStyle().
		Faint(true).
		Render(discoveryText)

	return fmt.Sprintf(
		"%s\n%s\n\n%s\n\n%s",
		header,
		discoveryView,
		m.viewport.View(),
		m.textarea.View(),
	) + "\n\n  ctrl+c to quit | Usage: <target> <method> [payload]"
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
	ta.Placeholder = "target method payload..."
	ta.Focus()
	ta.SetHeight(3)

	m := model{
		client:   client,
		textarea: ta,
		msgCh:    msgCh,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
