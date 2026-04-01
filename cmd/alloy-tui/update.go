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
		cmds = append(cmds, m.sendViewStateUpdate())

	case modal.MoveIntent:
		switch it.Direction {
		case "down", "up", "left", "right", "line-start", "line-end", "buffer-start", "buffer-end", "page-up", "page-down":
			if m.Mode == tui.ModeInsert || m.Mode == tui.ModeEdit || m.Mode == tui.ModeChat {
				// To avoid duplicate Update calls on msg, we can just call our own movement logic
				// (or re-map to msg)
				keyMsg := tea.KeyMsg{}
				switch it.Direction {
				case "down":
					keyMsg = tea.KeyMsg{Type: tea.KeyDown}
				case "up":
					keyMsg = tea.KeyMsg{Type: tea.KeyUp}
				case "left":
					keyMsg = tea.KeyMsg{Type: tea.KeyLeft}
				case "right":
					keyMsg = tea.KeyMsg{Type: tea.KeyRight}
				case "line-start":
					keyMsg = tea.KeyMsg{Type: tea.KeyHome}
				case "line-end":
					keyMsg = tea.KeyMsg{Type: tea.KeyEnd}
				case "buffer-start":
					m.textarea.SetCursor(0)
				case "buffer-end":
					lines := strings.Split(m.textarea.Value(), "\n")
					m.textarea.SetCursor(len(lines) - 1)
				}
				if keyMsg.Type != 0 {
					var taCmd tea.Cmd
					m.textarea, taCmd = m.textarea.Update(keyMsg)
					if taCmd != nil {
						cmds = append(cmds, taCmd)
					}
				}
				cmds = append(cmds, m.sendCursorUpdate(m.activeBuffer, m.textarea.Line(), 0))
			} else {
				// Viewport movement for Normal/Dashboard/Inspector
				keyMsg := tea.KeyMsg{}
				switch it.Direction {
				case "down":
					keyMsg = tea.KeyMsg{Type: tea.KeyDown}
				case "up":
					keyMsg = tea.KeyMsg{Type: tea.KeyUp}
				case "page-up":
					keyMsg = tea.KeyMsg{Type: tea.KeyPgUp}
				case "page-down":
					keyMsg = tea.KeyMsg{Type: tea.KeyPgDown}
				}
				if keyMsg.Type != 0 {
					var vpCmd tea.Cmd
					if m.Mode == tui.ModeInspector {
						m.inspectorVp, vpCmd = m.inspectorVp.Update(keyMsg)
					} else {
						m.viewport, vpCmd = m.viewport.Update(keyMsg)
					}
					if vpCmd != nil {
						cmds = append(cmds, vpCmd)
					}
				}
			}
		}

	case modal.WindowIntent:
		switch it.Action {
		case "split-v":
			m.splitFocusedPane("horizontal")
		case "split-h":
			m.splitFocusedPane("vertical")
		case "close":
			m.closeFocusedPane()
		case "focus-left", "focus-right", "focus-up", "focus-down":
			m.navigateFocus(it.Action[6:])
		case "focus-next":
			m.navigateFocus("right")
		case "focus-prev":
			m.navigateFocus("left")
		}
		cmds = append(cmds, m.sendLayoutUpdate(), m.sendViewStateUpdate())

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

	case modal.InputIntent:
		switch m.Mode {
		case tui.ModeInsert, tui.ModeEdit, tui.ModeChat:
			// Inject key into textarea
			// Since we're 100% intent based, we need to handle special cases in input
			var taCmd tea.Cmd
			keyMsg := tea.KeyMsg{Runes: []rune(it.Text), Type: tea.KeyRunes}
			if it.Text == "enter" {
				keyMsg = tea.KeyMsg{Type: tea.KeyEnter}
			} else if it.Text == "backspace" {
				keyMsg = tea.KeyMsg{Type: tea.KeyBackspace}
			} else if it.Text == "tab" {
				keyMsg = tea.KeyMsg{Type: tea.KeyTab}
			} else if it.Text == " " {
				keyMsg = tea.KeyMsg{Runes: []rune(" "), Type: tea.KeySpace}
			} else if len(it.Text) > 1 {
				// Special key not caught? fallback
				return m, nil
			}
			m.textarea, taCmd = m.textarea.Update(keyMsg)
			if taCmd != nil {
				cmds = append(cmds, taCmd)
			}
			if m.Mode == tui.ModeEdit {
				m.isLocalBufferDirty = true
				cmds = append(cmds, m.sendCursorUpdate(m.activeBuffer, m.textarea.Line(), 0))
			}
			if m.Mode == tui.ModeChat && it.Text == "enter" {
				val := m.textarea.Value()
				if strings.TrimSpace(val) != "" {
					cmds = append(cmds, m.sendChatMessage(val))
					m.textarea.SetValue("")
				}
			}
		case tui.ModeCommand, tui.ModeOmni:
			input := m.commandInput.Value()

			// Handle Leader mode sequences (single keys when input is empty)
			if m.isLeader && len(it.Text) == 1 && input == "" {
				if it.Text == ":" {
					m.isLeader = false
					m.breadcrumbs = nil
					m.commandInput.SetValue(":")
					m.selectedCmdIdx = 0
					return m, nil
				}

				node := m.commandTree.Find(m.breadcrumbs)
				if node != nil {
					char := it.Text
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

			// Navigation and standard input
			if it.Text == "down" {
				filteredCount := len(m.filteredCommands())
				if filteredCount > 0 {
					m.selectedCmdIdx = (m.selectedCmdIdx + 1) % filteredCount
				}
				return m, nil
			} else if it.Text == "up" {
				filteredCount := len(m.filteredCommands())
				if filteredCount > 0 {
					m.selectedCmdIdx = (m.selectedCmdIdx - 1 + filteredCount) % filteredCount
				}
				return m, nil
			} else if it.Text == "enter" {
				filtered := m.filteredCommands()
				if len(filtered) > 0 && m.selectedCmdIdx >= 0 && m.selectedCmdIdx < len(filtered) {
					opt := filtered[m.selectedCmdIdx]
					// Special selection modes (project/workspace list)
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

					// Standard command execution from filtered list
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

				// Global enter fallback
				rawCmd := m.commandInput.Value()
				m.commandInput.SetValue("")
				m.Mode = m.lastMainMode
				m.isLeader = false
				m.breadcrumbs = nil
				return m.executeCommand(rawCmd)
			} else if it.Text == "backspace" && m.isLeader && input == "" {
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

			// Normal text injection
			keyMsg := tea.KeyMsg{Runes: []rune(it.Text), Type: tea.KeyRunes}
			if it.Text == "backspace" {
				keyMsg = tea.KeyMsg{Type: tea.KeyBackspace}
			} else if it.Text == " " {
				keyMsg = tea.KeyMsg{Runes: []rune(" "), Type: tea.KeySpace}
			} else if len(it.Text) > 1 {
				return m, nil
			}
			m.commandInput, _ = m.commandInput.Update(keyMsg)
			if m.Mode == tui.ModeOmni {
				cmds = append(cmds, m.doOmniSearch(m.commandInput.Value()))
			}
		}

	case modal.ActionIntent:
		switch it.Verb {
		case "save":
			cmds = append(cmds, m.sendBufferUpdate(m.activeBuffer, m.textarea.Value(), false))
		case "leader-mode":
			m.lastMainMode = m.Mode
			m.Mode = tui.ModeCommand
			m.isLeader = true
			m.commandInput.SetValue("")
			m.commandInput.Focus()
		case "command-mode":
			m.lastMainMode = m.Mode
			m.Mode = tui.ModeCommand
			m.isLeader = false
			m.commandInput.SetValue(":")
			m.commandInput.Focus()
		case "insert-mode":
			if m.hasCapability("buffer:write") || m.hasCapability("ui:view:editor") {
				m.Mode = tui.ModeInsert
				m.textarea.Focus()
			} else {
				m.messages = append(m.messages, lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("!! Editor service (buffer) not available."))
			}
		case "chat-mode":
			if m.hasCapability("chat:send") || m.hasCapability("ui:view:chat") {
				m.Mode = tui.ModeChat
				m.textarea.Placeholder = "Type message to #" + m.ActiveChannel + "..."
				m.textarea.Focus()
			} else {
				m.messages = append(m.messages, lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("!! Chat service not available."))
			}
		case "dashboard-mode":
			m.Mode = tui.ModeDashboard
			m.isLeader = false
		case "inspector-mode":
			m.Mode = tui.ModeInspector
			m.isLeader = false
		case "omni-mode":
			m.lastMainMode = m.Mode
			m.Mode = tui.ModeOmni
			m.isLeader = false
			m.commandInput.SetValue("")
			m.commandInput.Focus()
			m.omniResults = nil
			m.omniSelectedIdx = 0
			cmds = append(cmds, m.doOmniSearch(""))
		}
		cmds = append(cmds, m.sendViewStateUpdate())
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
			Ctrl:  msg.Type >= tea.KeyCtrlA && msg.Type <= tea.KeyCtrlZ && msg.Type != tea.KeyTab && msg.Type != tea.KeyEnter && msg.Type != tea.KeyEsc,
			Shift: msg.Type == tea.KeyShiftTab,
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
		// If key was not consumed by ModalEngine, just drop it or handle global
		// keys that shouldn't be modalized.
		if consumed {
			return m, nil
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
					m.subscribe("system:context-changed"),
					m.subscribe("system:theme-changed"),
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

	// Mode is synced within handlers now
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
