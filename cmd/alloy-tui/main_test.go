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
	m.Panes = []tui.Pane{{Type: tui.ModeDashboard, WidthPct: 1.0}}
	m.FocusIdx = 0

	// 1. Vertical split
	model, _ := m.executeCommand("vsplit")
	m = model.(Model)

	if len(m.Panes) != 2 {
		t.Fatalf("Expected 2 panes after vsplit, got %d", len(m.Panes))
	}
	if m.Panes[0].WidthPct != 0.5 || m.Panes[1].WidthPct != 0.5 {
		t.Errorf("Expected equal width split (0.5), got %f and %f", m.Panes[0].WidthPct, m.Panes[1].WidthPct)
	}
	if m.FocusIdx != 1 {
		t.Errorf("Expected new pane to be focused (idx 1), got %d", m.FocusIdx)
	}
	if m.Mode != tui.ModeChat {
		t.Errorf("Expected new pane mode to be ModeChat, got %d", m.Mode)
	}

	// 2. Focus next
	model, _ = m.executeCommand("focus-next")
	m = model.(Model)
	if m.FocusIdx != 0 {
		t.Errorf("Expected focus to wrap to 0, got %d", m.FocusIdx)
	}
	if m.Mode != tui.ModeDashboard {
		t.Errorf("Expected mode to switch to ModeDashboard, got %d", m.Mode)
	}

	// 3. Close pane
	model, _ = m.executeCommand("close-pane")
	m = model.(Model)
	if len(m.Panes) != 1 {
		t.Fatalf("Expected 1 pane after close-pane, got %d", len(m.Panes))
	}
	if m.FocusIdx != 0 {
		t.Errorf("Expected focus 0, got %d", m.FocusIdx)
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
	projectPayload := `{"topic":"project:opened","data":{"id":"proj-1","name":"Test Project","layout":{"layout":[{"type":"editor","width_pct":0.7},{"type":"chat","width_pct":0.3}]}}}`
	msg := api.Message{
		Sender:  "events",
		Method:  "project:opened",
		Payload: []byte(projectPayload),
	}

	m.processMessage(msg)

	if len(m.Panes) != 2 {
		t.Fatalf("Expected 2 panes, got %d", len(m.Panes))
	}
	if m.Panes[0].Type != tui.ModeEdit || m.Panes[1].Type != tui.ModeChat {
		t.Errorf("Expected panes [Edit, Chat], got [%d, %d]", m.Panes[0].Type, m.Panes[1].Type)
	}
	if m.Mode != tui.ModeEdit {
		t.Errorf("Expected initial mode to be Edit, got %d", m.Mode)
	}

	// 2. Simulate workspace:opened with override layout
	workspacePayload := `{"topic":"workspace:opened","data":{"id":"ws-1","name":"Test Workspace","layout":"{\"layout\":[{\"type\":\"dashboard\",\"width_pct\":1.0}]}"}}`
	msg = api.Message{
		Sender:  "events",
		Method:  "workspace:opened",
		Payload: []byte(workspacePayload),
	}

	m.processMessage(msg)

	if len(m.Panes) != 1 {
		t.Fatalf("Expected 1 pane after workspace override, got %d", len(m.Panes))
	}
	if m.Panes[0].Type != tui.ModeDashboard {
		t.Errorf("Expected pane mode Dashboard, got %d", m.Panes[0].Type)
	}
	if m.Mode != tui.ModeDashboard {
		t.Errorf("Expected mode to switch to Dashboard, got %d", m.Mode)
	}
}
