package tui

import (
	"testing"

	"github.com/james-nesbitt/alloy/pkg/frontend"
)

func TestFlattenLayout(t *testing.T) {
	root := &frontend.LayoutNode{
		Type:      "split",
		Direction: "horizontal",
		Children: []frontend.LayoutNode{
			{
				Type:     "pane",
				Mode:     "dashboard",
				Weight:   0.5,
				PluginID: "p1",
			},
			{
				Type:      "split",
				Direction: "vertical",
				Weight:    0.5,
				Children: []frontend.LayoutNode{
					{
						Type:     "pane",
						Mode:     "chat",
						Weight:   0.5,
						PluginID: "p2",
					},
					{
						Type:     "pane",
						Mode:     "editor",
						Weight:   0.5,
						PluginID: "p3",
					},
				},
			},
		},
	}

	panes := FlattenLayout(root)

	if len(panes) != 3 {
		t.Fatalf("Expected 3 panes, got %d", len(panes))
	}

	if panes[0].Type != ModeDashboard || panes[0].WidgetID != "p1" {
		t.Errorf("Pane 0 mismatch: %+v", panes[0])
	}
	if panes[1].Type != ModeChat || panes[1].WidgetID != "p2" {
		t.Errorf("Pane 1 mismatch: %+v", panes[1])
	}
	if panes[2].Type != ModeEdit || panes[2].WidgetID != "p3" {
		t.Errorf("Pane 2 mismatch: %+v", panes[2])
	}
}

func TestFlattenLayoutLegacy(t *testing.T) {
	// Root is a single pane
	root := &frontend.LayoutNode{
		Type:     "pane",
		Mode:     "dashboard",
		PluginID: "main",
	}

	panes := FlattenLayout(root)
	if len(panes) != 1 {
		t.Fatalf("Expected 1 pane, got %d", len(panes))
	}
	if panes[0].Type != ModeDashboard || panes[0].WidgetID != "main" {
		t.Errorf("Pane mismatch: %+v", panes[0])
	}
}
