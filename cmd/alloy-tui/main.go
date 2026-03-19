package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/ipc"
	"github.com/jnesbitt/alloy-go/pkg/security/identity"
)

const (
	XDG_CONFIG_HOME = "XDG_CONFIG_HOME"
	XDG_RUNTIME_DIR = "XDG_RUNTIME_DIR"
)

func getAlloyHome() string {
	if home := os.Getenv(XDG_CONFIG_HOME); home != "" {
		return filepath.Join(home, "alloy")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "alloy")
}

func getAlloyRuntimeDir() string {
	if run := os.Getenv(XDG_RUNTIME_DIR); run != "" {
		return filepath.Join(run, "alloy")
	}
	return filepath.Join(os.TempDir(), "alloy")
}

// Model represents the state of our TUI application.
type model struct {
	client    *ipc.Client
	messages  []string
	viewport  viewport.Model
	textarea  textarea.Model
	err       error
	width     int
	height    int
	ready     bool
	connected bool
	subCh     chan api.Message
}

type messageMsg api.Message
type errMsg error

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.listenForMessages(),
	)
}

func (m model) listenForMessages() tea.Cmd {
	return func() tea.Msg {
		msg := <-m.subCh
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
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			content := m.textarea.Value()
			if content == "" {
				return m, nil
			}
			
			// For now, assume simple "target:method payload"
			parts := strings.SplitN(content, " ", 3)
			if len(parts) >= 2 {
				target := parts[0]
				method := parts[1]
				payload := ""
				if len(parts) == 3 {
					payload = parts[2]
				}

				sendMsg := api.Message{
					ID:        fmt.Sprintf("tui-%d", time.Now().UnixNano()),
					Type:      api.TypeRequest,
					Sender:    "alloy-tui",
					Target:    target,
					Method:    method,
					Payload:   json.RawMessage(payload),
					Timestamp: time.Now().Unix(),
				}

				m.messages = append(m.messages, lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Render("-> "+content))
				m.textarea.Reset()
				m.viewport.SetContent(strings.Join(m.messages, "\n"))
				m.viewport.GotoBottom()

				return m, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					resp, err := m.client.Call(ctx, sendMsg)
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
			m.viewport = viewport.New(msg.Width, msg.Height-5)
			m.viewport.HighPerformanceRendering = false
			m.viewport.SetContent("Connected to Alloy Core. Type 'target method payload' to send a message.\nExample: kernel ping")
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 5
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
	return fmt.Sprintf(
		"%s\n\n%s",
		m.viewport.View(),
		m.textarea.View(),
	) + "\n\n  ctrl+c to quit"
}

func main() {
	name := flag.String("name", "alloy-tui", "Name of the TUI component")
	socket := flag.String("socket", filepath.Join(getAlloyRuntimeDir(), "default.sock"), "Socket address of the core")
	insecure := flag.Bool("insecure", false, "Disable mTLS")
	flag.Parse()

	var tlsConfig *tls.Config
	if !*insecure {
		store := identity.NewStore(getAlloyHome())
		ca, err := store.InitializeMachine()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		tlsConfig, err = store.GetClientTLSConfig(ca, *name)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	}

	client, err := ipc.Dial(*socket, tlsConfig)
	if err != nil {
		fmt.Printf("Failed to connect to core: %v\n", err)
		os.Exit(1)
	}

	subCh := make(chan api.Message)
	go func() {
		async := client.Async()
		for {
			msg := <-async
			subCh <- msg
		}
	}()

	ta := textarea.New()
	ta.Placeholder = "target method payload..."
	ta.Focus()
	ta.CharLimit = 1000
	ta.SetHeight(3)

	m := model{
		client:    client,
		textarea:  ta,
		connected: true,
		subCh:     subCh,
	}

	// Bootstrap: tell core to treat us as a frontend
	// ... (This is actually handled by the server.go:handleConn:RegisterFrontend)
	
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
