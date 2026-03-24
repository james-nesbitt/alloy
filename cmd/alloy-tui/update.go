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
		interval := time.Second * 5
		if m.startupTicks < 10 {
			interval = time.Millisecond * 500
			m.startupTicks++
		}
		cmds = append(cmds,
			tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) }),
			m.doDiscovery,
			m.sendPresenceHeartbeat(),
		)

	case tea.KeyMsg:
		// Global quit
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}

		// Handle keys based on mode
		switch m.Mode {
		case tui.ModeNormal, tui.ModeDashboard:
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

		case tui.ModeInsert:
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

		case tui.ModeChat:
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

		case tui.ModeEdit:
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

		case tui.ModeCommand:
			newM, cmd := m.handleCommandMode(msg, nil)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			m = newM.(Model)

		case tui.ModeForm:
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

		// Re-fetch widgets if we have the capability but no tiles
		if len(m.DashboardTiles) == 0 && m.hasCapability("dashboard:list-widgets") {
			cmds = append(cmds, m.fetchInitialWidgets())
		}

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

func (m Model) doDiscovery() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := m.client.Send(ctx, "command-manager", "discover", nil)
	if err != nil {
		return errMsg(fmt.Errorf("discovery failed: %w", err))
	}
	var dMsg discoveryMsg
	if err := json.Unmarshal(resp.Payload, &dMsg); err != nil {
		return errMsg(fmt.Errorf("discovery unmarshal failed: %w", err))
	}
	return dMsg
}
