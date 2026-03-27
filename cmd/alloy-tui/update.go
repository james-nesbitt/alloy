package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
	"github.com/james-nesbitt/alloy/pkg/frontend/modal"
	"github.com/james-nesbitt/alloy/pkg/frontend/tui"
)

func (m Model) dispatchIntent(intent modal.Intent) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch it := intent.(type) {
	case modal.ModeIntent:
		switch it.NewMode {
		case modal.ModeNormal:
			m.Mode = tui.ModeNormal
			m.textarea.Blur()
		case modal.ModeInsert:
			// If we are in edit mode, just stay in edit mode's insert
			// If in normal, switch to insert or edit
			if m.Mode == tui.ModeNormal || m.Mode == tui.ModeDashboard {
				if m.activeBuffer != "" {
					m.Mode = tui.ModeEdit
				} else {
					m.Mode = tui.ModeInsert
				}
			}
			m.textarea.Focus()
		case modal.ModeCommand:
			m.lastMainMode = m.Mode
			m.Mode = tui.ModeCommand
			m.commandInput.Focus()
		}

	case modal.MoveIntent:
		switch it.Direction {
		case "down":
			m.textarea.SetCursor(m.textarea.Line() + 1)
			cmds = append(cmds, m.sendCursorUpdate(m.activeBuffer, m.textarea.Line(), 0))
		case "up":
			m.textarea.SetCursor(m.textarea.Line() - 1)
			cmds = append(cmds, m.sendCursorUpdate(m.activeBuffer, m.textarea.Line(), 0))
		case "buffer-start":
			m.textarea.SetCursor(0)
			cmds = append(cmds, m.sendCursorUpdate(m.activeBuffer, 0, 0))
		case "buffer-end":
			lines := strings.Split(m.textarea.Value(), "\n")
			m.textarea.SetCursor(len(lines) - 1)
			cmds = append(cmds, m.sendCursorUpdate(m.activeBuffer, len(lines)-1, 0))
		}

	case modal.WindowIntent:
		switch it.Action {
		case "split-v":
			// Simple append logic
			m.Panes = append(m.Panes, tui.Pane{Type: m.Mode, WidthPct: 0.5})
			// Adjust others? For now just append
		case "close":
			if len(m.Panes) > 1 {
				m.Panes = append(m.Panes[:m.FocusIdx], m.Panes[m.FocusIdx+1:]...)
				m.FocusIdx = m.FocusIdx % len(m.Panes)
				m.Mode = m.Panes[m.FocusIdx].Type
			}
		case "focus-left":
			m.FocusIdx = (m.FocusIdx - 1 + len(m.Panes)) % len(m.Panes)
			m.Mode = m.Panes[m.FocusIdx].Type
		case "focus-right":
			m.FocusIdx = (m.FocusIdx + 1) % len(m.Panes)
			m.Mode = m.Panes[m.FocusIdx].Type
		}

	case modal.BufferIntent:
		switch it.Action {
		case "save":
			cmds = append(cmds, m.sendBufferUpdate(m.activeBuffer, m.textarea.Value(), false))
		case "fuzzy-find":
			m.lastMainMode = m.Mode
			m.Mode = tui.ModeOmni
			m.commandInput.Focus()
			cmds = append(cmds, m.doOmniSearch(""))
		}

	case modal.SearchIntent:
		m.lastMainMode = m.Mode
		m.Mode = tui.ModeOmni
		m.commandInput.Focus()
		cmds = append(cmds, m.doOmniSearch(""))

	case modal.ActionIntent:
		switch it.Verb {
		case "save":
			cmds = append(cmds, m.sendBufferUpdate(m.activeBuffer, m.textarea.Value(), false))
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 3
		m.inspectorVp.Width = msg.Width
		m.inspectorVp.Height = msg.Height - 3
		m.textarea.SetWidth(msg.Width)
		m.commandInput.SetWidth(msg.Width)
		m.omniList.SetSize(msg.Width-4, msg.Height/2)
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

		// PROCESS MODAL SYSTEM
		mKey := modal.Key{
			Code:  msg.String(),
			Alt:   msg.Alt,
			Ctrl:  msg.Type >= tea.KeyCtrlA && msg.Type <= tea.KeyCtrlZ,
			Shift: msg.Type == tea.KeyShiftTab, // simplified
		}

		// Map special keys to consistent codes
		switch msg.Type {
		case tea.KeyEsc:
			mKey.Code = "esc"
		case tea.KeyEnter:
			mKey.Code = "enter"
		case tea.KeySpace:
			mKey.Code = " "
		case tea.KeyBackspace:
			mKey.Code = "backspace"
		case tea.KeyTab:
			mKey.Code = "tab"
		}

		// If it's a Ctrl+ char, the String() is "ctrl+X". Let's also set Code to "X" if Ctrl is true
		if mKey.Ctrl && strings.HasPrefix(mKey.Code, "ctrl+") {
			mKey.Code = strings.TrimPrefix(mKey.Code, "ctrl+")
		}

		// Sync current model mode into engine
		switch m.Mode {
		case tui.ModeNormal, tui.ModeDashboard, tui.ModeInspector:
			m.ModalEngine.State.Mode = modal.ModeNormal
		case tui.ModeInsert, tui.ModeEdit, tui.ModeChat:
			m.ModalEngine.State.Mode = modal.ModeInsert
		case tui.ModeCommand, tui.ModeForm, tui.ModeOmni:
			m.ModalEngine.State.Mode = modal.ModeCommand
		}

		intent, consumed := m.ModalEngine.Process(mKey)
		if intent != nil {
			var cmd tea.Cmd
			newModel, cmd := m.dispatchIntent(intent)
			m = newModel.(Model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		if consumed {
			return m, nil
		}

		// Handle keys based on mode (Legacy fallback)
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

		case tui.ModeOmni:
			newM, cmd := m.handleOmniMode(msg, nil)
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
					m.subscribe("system:trace"),
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

	case []OmniResult:
		m.omniResults = msg
		items := make([]list.Item, len(msg))
		for i, res := range msg {
			items[i] = listItem{res: res}
		}
		m.omniList.SetItems(items)
		return m, nil

	case searchDebounceMsg:
		if msg.Query == m.commandInput.Value() {
			return m, m.doOmniSearch(msg.Query)
		}
		return m, nil

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
