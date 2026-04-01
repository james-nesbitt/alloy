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

// SplitNode wraps the node with the given ID in a new split.
func SplitNode(root *frontend.LayoutNode, id string, direction string) *frontend.LayoutNode {
	node := FindNodeByID(root, id)
	if node == nil {
		return nil
	}

	newNodeID := "pane-" + id + "-split" // Simplified, callers might want better IDs

	oldNode := *node
	node.Type = "split"
	node.Direction = direction
	node.PluginID = ""
	node.Mode = ""
	node.ID = ""
	node.Children = []frontend.LayoutNode{
		oldNode,
		{
			ID:     newNodeID,
			Type:   "pane",
			Mode:   oldNode.Mode,
			Weight: 0.5,
		},
	}
	node.Children[0].Weight = 0.5

	return &node.Children[1]
}

// RemoveNode removes the node with the given ID from the tree and collapses parent splits if necessary.
func RemoveNode(root **frontend.LayoutNode, id string) bool {
	if *root == nil {
		return false
	}

	if (*root).ID == id {
		// Cannot remove root node if it's the only one
		return false
	}

	var removeRecursive func(*frontend.LayoutNode) bool
	removeRecursive = func(node *frontend.LayoutNode) bool {
		if node.Type != "split" {
			return false
		}
		for i, child := range node.Children {
			if child.ID == id {
				node.Children = append(node.Children[:i], node.Children[i+1:]...)
				return true
			}
			if removeRecursive(&node.Children[i]) {
				// If child became an empty split, remove it
				if node.Children[i].Type == "split" && len(node.Children[i].Children) == 0 {
					node.Children = append(node.Children[:i], node.Children[i+1:]...)
				}
				return true
			}
		}
		return false
	}

	if removeRecursive(*root) {
		// Simplify root split if it only has one child
		if (*root).Type == "split" && len((*root).Children) == 1 {
			*root = &(*root).Children[0]
		}
		return true
	}

	return false
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

// NavigateFocus returns the ID of the next/previous pane in the tree.
func NavigateFocus(root *frontend.LayoutNode, currentID string, direction int) string {
	panes := GetPanes(root)
	if len(panes) == 0 {
		return ""
	}
	if len(panes) == 1 {
		return panes[0].ID
	}

	idx := -1
	for i, p := range panes {
		if p.ID == currentID {
			idx = i
			break
		}
	}

	if idx == -1 {
		return panes[0].ID
	}

	newIdx := (idx + direction + len(panes)) % len(panes)
	return panes[newIdx].ID
}
