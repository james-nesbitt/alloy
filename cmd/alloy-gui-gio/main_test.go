//go:build gui

package main

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"image"
	"testing"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

func TestDrawDashboard(t *testing.T) {
	gtx := layout.Context{
		Ops: new(op.Ops),
		Constraints: layout.Constraints{
			Max: image.Pt(800, 600),
		},
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	th := material.NewTheme()

	state := guiState{
		dashboardTiles: make(map[string]frontend.DashboardTile),
		tileOrder:      []string{"test"},
	}
	state.dashboardTiles["test"] = frontend.DashboardTile{
		Title:   "Test Tile",
		Content: []string{"Line 1", "Line 2"},
		Status:  "OK",
	}

	dims := drawDashboard(gtx, th, &state)

	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Errorf("Expected positive dimensions, got %v", dims.Size)
	}
}

func TestHandleEventModeSwitch(t *testing.T) {
	state := &guiState{
		mode: ModeNormal,
	}
	input := &widget.Editor{}
	gtx := layout.Context{} // No ops, so gtx.Execute is a no-op with nil gtx.Ops

	// Test switching to command mode with ":"
	state.handleEvent(key.Event{Name: ":", State: key.Press}, input, nil, nil, gtx)
	if state.mode != ModeCommand {
		t.Errorf("Expected mode to be ModeCommand, got %d", state.mode)
	}
	if state.isLeader {
		t.Errorf("Expected isLeader to be false")
	}

	// Reset
	state.mode = ModeNormal

	// Test switching to leader mode with space
	state.handleEvent(key.Event{Name: key.NameSpace, State: key.Press}, input, nil, nil, gtx)
	if state.mode != ModeCommand {
		t.Errorf("Expected mode to be ModeCommand, got %d", state.mode)
	}
	if !state.isLeader {
		t.Errorf("Expected isLeader to be true")
	}
}

func TestBuildCommandTreeDeep(t *testing.T) {
	regs := []api.Registration{
		{
			ID: "test-plugin",
			Capabilities: []api.Capability{
				{Method: "test:hello", Description: "Say hello", Shortcut: "h e"},
				{Method: "test:world", Description: "Say world", Shortcut: "w"},
			},
		},
	}

	tree := frontend.BuildCommandTree(regs)

	// Check shortcut "h e"
	hNode, ok := tree.Children["h"]
	if !ok {
		t.Fatal("Missing node 'h' for shortcut")
	}
	eNode, ok := hNode.Children["e"]
	if !ok {
		t.Fatal("Missing node 'e' for shortcut 'h e'")
	}
	if eNode.Method != "test:hello" {
		t.Errorf("Expected method 'test:hello', got %q", eNode.Method)
	}

	// Check shortcut "w"
	wNode, ok := tree.Children["w"]
	if !ok {
		t.Fatal("Missing node 'w' for shortcut")
	}
	if wNode.Method != "test:world" {
		t.Errorf("Expected method 'test:world', got %q", wNode.Method)
	}

	// Check default ":test-plugin"
	pluginNode, ok := tree.Children["test-plugin"]
	if !ok {
		t.Fatal("Missing node 'test-plugin' for default discovery")
	}
	helloNode, ok := pluginNode.Children["hello"]
	if !ok {
		t.Fatal("Missing node 'hello' under 'test-plugin'")
	}
	if helloNode.Method != "test:hello" {
		t.Errorf("Expected method 'test:hello', got %q", helloNode.Method)
	}
}

func TestGuiLeaderDrillDown(t *testing.T) {
	regs := []api.Registration{
		{
			ID: "test",
			Capabilities: []api.Capability{
				{Method: "test:hello", Description: "Say hello", Shortcut: "h e"},
			},
		},
	}
	state := &guiState{
		mode:        ModeCommand,
		commandTree: frontend.BuildCommandTree(regs),
		isLeader:    true,
		breadcrumbs: nil,
	}
	input := &widget.Editor{}
	gtx := layout.Context{}

	// Press 'h'
	state.handleEvent(key.Event{Name: "h", State: key.Press}, input, nil, nil, gtx)
	if len(state.breadcrumbs) != 1 || state.breadcrumbs[0] != "h" {
		t.Errorf("Expected breadcrumbs ['h'], got %v", state.breadcrumbs)
	}

	// Press 'e' (this should trigger execution if it was a leaf, but it's a leaf here)
	// Actually, "h e" shortcut:
	// 'h' -> node with child 'e'
	// 'e' -> node with method "test:hello"
	state.handleEvent(key.Event{Name: "e", State: key.Press}, input, nil, nil, gtx)

	// Since executeCommand is just a placeholder and I passed nil client/window,
	// it might have crashed if I didn't handle it.
	// But in handleEvent: if child is leaf, it calls executeCommand AND resets state.
	if state.mode != ModeNormal {
		t.Errorf("Expected mode to be ModeNormal after execution, got %d", state.mode)
	}
	if state.isLeader {
		t.Error("Expected isLeader to be false after execution")
	}
}
