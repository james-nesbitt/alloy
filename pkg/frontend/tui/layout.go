package tui

import (
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

// FlattenLayout recursively traverses the LayoutNode tree and returns a flat list of Panes.
// This is used for backward compatibility with the current TUI and GUI rendering logic.
func FlattenLayout(node *frontend.LayoutNode) []Pane {
	if node == nil {
		return nil
	}
	if node.Type == "pane" {
		p := Pane{
			WidthPct: node.Weight,
			WidgetID: node.PluginID,
		}
		if p.WidthPct == 0 {
			p.WidthPct = 1.0
		}
		switch node.Mode {
		case "dashboard":
			p.Type = ModeDashboard
		case "chat":
			p.Type = ModeChat
		case "editor":
			p.Type = ModeEdit
		case "inspector":
			p.Type = ModeInspector
		default:
			p.Type = ModeNormal
		}
		return []Pane{p}
	}
	var panes []Pane
	for _, child := range node.Children {
		panes = append(panes, FlattenLayout(&child)...)
	}

	// For now, in flat mode, we just sum up the widths and re-scale
	// This is a naive translation of a nested tree to a flat list of panes.
	return panes
}

// FindNodeByID recursively searches for a LayoutNode with the given ID.
func FindNodeByID(root *frontend.LayoutNode, id string) *frontend.LayoutNode {
	if root == nil {
		return nil
	}
	if root.ID == id {
		return root
	}
	for i := range root.Children {
		if found := FindNodeByID(&root.Children[i], id); found != nil {
			return found
		}
	}
	return nil
}

// GetPanes returns a flat list of pointer to all nodes of type "pane".
func GetPanes(root *frontend.LayoutNode) []*frontend.LayoutNode {
	if root == nil {
		return nil
	}
	if root.Type == "pane" {
		return []*frontend.LayoutNode{root}
	}
	var panes []*frontend.LayoutNode
	for i := range root.Children {
		panes = append(panes, GetPanes(&root.Children[i])...)
	}
	return panes
}
