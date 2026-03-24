package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
	"testing"
)

func TestTuiLeaderModeNavigation(t *testing.T) {
	// 1. Initialize a model with a mock client
	m := NewModel(nil, nil) 
	m.ready = true
	m.Mode = ModeNormal // Explicitly set to Normal
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

	if m.Mode != ModeCommand {
		t.Errorf("Expected ModeCommand, got %d", m.Mode)
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

	if m.Mode != ModeNormal {
		t.Errorf("Expected ModeNormal after ESC, got %d", m.Mode)
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
	m.Mode = ModeCommand
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
