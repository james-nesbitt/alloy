package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
	"github.com/james-nesbitt/alloy/pkg/frontend/tui"
)

func TestTuiLeaderModeNavigation(t *testing.T) {
	// 1. Initialize a model with a mock client
	m := NewModel(nil, nil)
	m.ready = true
	m.Mode = tui.ModeNormal // Explicitly set to Normal
	m.width = 80
	m.height = 24

	// Manually inject a command tree
	m.commandTree = frontend.BuildCommandTree([]api.Registration{
		{
			ID: "project",
			Capabilities: []api.Capability{
				{Method: "list-workspaces", Description: "List all workspaces", Shortcut: "p p"},
			},
		},
	})

	// 2. Simulate "SPC" to enter Leader mode
	msg := tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	model, _ := m.Update(msg)
	m = model.(Model)

	if m.Mode != tui.ModeCommand {
		t.Errorf("Expected tui.ModeCommand, got %d", m.Mode)
	}
	if !m.isLeader {
		t.Error("Expected isLeader to be true")
	}

	// 3. Simulate "p" to drill down into project
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}
	model, _ = m.Update(msg)
	m = model.(Model)

	if len(m.breadcrumbs) != 1 || m.breadcrumbs[0] != "p" {
		t.Errorf("Expected breadcrumbs ['p'], got %v", m.breadcrumbs)
	}

	// 4. Simulate Backspace to go back
	msg = tea.KeyMsg{Type: tea.KeyBackspace}
	model, _ = m.Update(msg)
	m = model.(Model)

	if len(m.breadcrumbs) != 0 {
		t.Errorf("Expected empty breadcrumbs after backspace, got %v", m.breadcrumbs)
	}

	// 5. Simulate ESC to exit
	msg = tea.KeyMsg{Type: tea.KeyEsc}
	model, _ = m.Update(msg)
	m = model.(Model)

	if m.Mode != tui.ModeNormal {
		t.Errorf("Expected tui.ModeNormal after ESC, got %d", m.Mode)
	}
}

func TestTuiFuzzyMatching(t *testing.T) {
	m := NewModel(nil, nil)
	m.commandTree = frontend.BuildCommandTree([]api.Registration{
		{
			ID: "project",
			Capabilities: []api.Capability{
				{Method: "list-workspaces", Description: "Workspaces", Shortcut: "p p"},
			},
		},
	})
	m.Mode = tui.ModeCommand
	m.isLeader = false
	m.commandInput.SetValue("p p")

	filtered := m.filteredCommands()
	if len(filtered) == 0 {
		t.Fatal("Expected at least one filtered command for 'p p'")
	}

	found := false
	for _, opt := range filtered {
		if opt.Display == "p p" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'p p' in filtered results, got %+v", filtered)
	}
}

func TestTuiPaneManagement(t *testing.T) {
	m := NewModel(nil, nil)
	m.ready = true
	m.width = 100
	m.height = 30
	m.Mode = tui.ModeDashboard
	m.RootLayout = &frontend.LayoutNode{ID: "main", Type: "pane", Mode: "dashboard"}
	m.FocusedPaneID = "main"

	// 1. Vertical split
	m.splitFocusedPane("horizontal")

	panes := tui.GetPanes(m.RootLayout)
	if len(panes) != 2 {
		t.Fatalf("Expected 2 panes after split, got %d", len(panes))
	}
	if panes[0].Weight != 0.5 || panes[1].Weight != 0.5 {
		t.Errorf("Expected equal width split (0.5), got %f and %f", panes[0].Weight, panes[1].Weight)
	}
	if m.FocusedPaneID != panes[1].ID {
		t.Errorf("Expected new pane to be focused, got %s", m.FocusedPaneID)
	}

	// 2. Focus next (tab)
	msg := tea.KeyMsg{Type: tea.KeyTab}
	model, _ := m.Update(msg)
	m = model.(Model)
	if m.FocusedPaneID != panes[0].ID {
		t.Errorf("Expected focus to wrap to first pane, got %s", m.FocusedPaneID)
	}
	if m.Mode != tui.ModeDashboard {
		t.Errorf("Expected mode to switch to ModeDashboard, got %d", m.Mode)
	}

	// 3. Close pane
	m.closeFocusedPane()
	panes = tui.GetPanes(m.RootLayout)
	if len(panes) != 1 {
		t.Fatalf("Expected 1 pane after close, got %d", len(panes))
	}
}

func TestTuiOmniPalette(t *testing.T) {
	m := NewModel(nil, nil)
	m.ready = true
	m.commandTree = frontend.BuildCommandTree([]api.Registration{
		{
			ID: "ai",
			Capabilities: []api.Capability{
				{Method: "query", Description: "Ask AI", Shortcut: "a q"},
			},
		},
	})

	// 1. Initial State
	m.Mode = tui.ModeNormal

	// 2. ":" to enter Command mode
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
	model, _ := m.Update(msg) // handleNormalMode handles this
	m = model.(Model)

	if m.Mode != tui.ModeCommand || m.isLeader {
		t.Errorf("Expected ModeCommand and !isLeader, got mode %d, isLeader %v", m.Mode, m.isLeader)
	}
	if m.commandInput.Value() != ":" {
		t.Errorf("Expected command input to be ':', got '%s'", m.commandInput.Value())
	}

	// 3. Type "ai query hello" and Enter
	// We'll simulate this by setting value and sending Enter
	m.commandInput.SetValue(":ai query hello")
	msg = tea.KeyMsg{Type: tea.KeyEnter}
	model, _ = m.Update(msg)
	m = model.(Model)

	if m.Mode != tui.ModeNormal {
		t.Errorf("Expected ModeNormal after execution, got %d", m.Mode)
	}
}

func TestTuiCapabilityValidation(t *testing.T) {
	m := NewModel(nil, nil)
	m.ready = true
	m.Mode = tui.ModeNormal
	m.commandTree = frontend.BuildCommandTree([]api.Registration{}) // Empty tree

	// 1. Try to enter insert mode 'i' (requires buffer:write or ui:view:editor)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}
	model, _ := m.Update(msg)
	m = model.(Model)

	if m.Mode == tui.ModeInsert {
		t.Error("Expected NOT to enter ModeInsert when capability is missing")
	}
	foundError := false
	for _, msg := range m.messages {
		if strings.Contains(msg, "Editor service (buffer) not available") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Error("Expected error message for missing editor service")
	}

	// 2. Try to enter chat mode 'c' (requires chat:send or ui:view:chat)
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	model, _ = m.Update(msg)
	m = model.(Model)

	if m.Mode == tui.ModeChat {
		t.Error("Expected NOT to enter ModeChat when capability is missing")
	}
	foundError = false
	for _, msg := range m.messages {
		if strings.Contains(msg, "Chat service not available") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Error("Expected error message for missing chat service")
	}
}

func TestTuiLayoutApplication(t *testing.T) {
	m := NewModel(nil, nil)
	m.ready = true

	// 1. Simulate project:opened with custom layout
	projectPayload := `{"topic":"project:opened","data":{"id":"proj-1","name":"Test Project","layout":{"root":{"type":"split","direction":"horizontal","children":[{"id":"p1","type":"pane","mode":"editor","weight":0.7},{"id":"p2","type":"pane","mode":"chat","weight":0.3}]}}}}`
	msg := api.Message{
		Sender:  "events",
		Method:  "project:opened",
		Payload: []byte(projectPayload),
	}

	m.processMessage(msg)

	panes := tui.GetPanes(m.RootLayout)
	if len(panes) != 2 {
		t.Fatalf("Expected 2 panes, got %d", len(panes))
	}
	if panes[0].Mode != "editor" || panes[1].Mode != "chat" {
		t.Errorf("Expected panes [editor, chat], got [%s, %s]", panes[0].Mode, panes[1].Mode)
	}
	if m.Mode != tui.ModeEdit {
		t.Errorf("Expected initial mode to be Edit, got %d", m.Mode)
	}

	// 2. Simulate workspace:opened with override layout
	workspacePayload := `{"topic":"workspace:opened","data":{"id":"ws-1","name":"Test Workspace","layout":"{\"root\":{\"id\":\"p3\",\"type\":\"pane\",\"mode\":\"dashboard\",\"weight\":1.0}}"}}`
	msg = api.Message{
		Sender:  "events",
		Method:  "workspace:opened",
		Payload: []byte(workspacePayload),
	}

	m.processMessage(msg)

	panes = tui.GetPanes(m.RootLayout)
	if len(panes) != 1 {
		t.Fatalf("Expected 1 pane after workspace override, got %d", len(panes))
	}
	if panes[0].Mode != "dashboard" {
		t.Errorf("Expected pane mode dashboard, got %s", panes[0].Mode)
	}
	if m.Mode != tui.ModeDashboard {
		t.Errorf("Expected mode to switch to Dashboard, got %d", m.Mode)
	}
}
