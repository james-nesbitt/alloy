package frontend

import (
	"sort"
	"strings"

	"github.com/james-nesbitt/alloy/api"
)

// Registration is the frontend's view of a plugin's surface area.
type Registration struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Status       string           `json:"status,omitempty"`
	Capabilities []api.Capability `json:"capabilities,omitempty"`
}

// CommandNode is a node in a hierarchical command tree.
type CommandNode struct {
	Key         string
	Description string
	Target      string
	Method      string
	Children    map[string]*CommandNode
	Annotation  string
	Shortcut    string
}

// BuildCommandTree constructs a tree from a list of plugin registrations.
func BuildCommandTree(regs []Registration) *CommandNode {
	root := &CommandNode{Children: make(map[string]*CommandNode)}

	for _, r := range regs {
		for _, cap := range r.Capabilities {
			if cap.Shortcut == "" {
				continue
			}

			// Split by space for keystroke sequences
			keys := strings.Fields(cap.Shortcut)
			curr := root

			for i, k := range keys {
				if _, ok := curr.Children[k]; !ok {
					curr.Children[k] = &CommandNode{
						Key:      k,
						Children: make(map[string]*CommandNode),
					}
				}
				curr = curr.Children[k]

				if i == len(keys)-1 {
					curr.Description = cap.Description
					curr.Target = r.ID
					curr.Method = cap.Method
					curr.Shortcut = cap.Shortcut
					if group, ok := cap.Annotations["group"]; ok {
						curr.Annotation = group
					}
				}
			}
		}
	}
	return root
}

// Find locates a node by its sequential key path.
func (n *CommandNode) Find(path []string) *CommandNode {
	curr := n
	for _, k := range path {
		if child, ok := curr.Children[k]; ok {
			curr = child
		} else {
			return nil
		}
	}
	return curr
}

// SearchItem is a flattened representation of a command for fuzzy searching.
type SearchItem struct {
	FullTitle   string // e.g., "project open"
	Description string
	Shortcut    string
	Target      string
	Method      string
	Weight      int
	Frequency   int
	Status      string // "running", "crashed", etc.
}

// Flatten extracts all leaf commands from the tree.
func (n *CommandNode) Flatten(prefix string) []SearchItem {
	var results []SearchItem

	for _, child := range n.Children {
		newPrefix := child.Key
		if prefix != "" {
			newPrefix = prefix + " " + child.Key
		}

		if child.Method != "" {
			results = append(results, SearchItem{
				FullTitle:   newPrefix,
				Description: child.Description,
				Shortcut:    child.Shortcut,
				Target:      child.Target,
				Method:      child.Method,
			})
		}

		results = append(results, child.Flatten(newPrefix)...)
	}
	return results
}

// SortItems orders search results by weight (descending) and then lexicographically.
func SortItems(items []SearchItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Weight != items[j].Weight {
			return items[i].Weight > items[j].Weight
		}
		return items[i].FullTitle < items[j].FullTitle
	})
}

// FuzzyMatch performs a simple case-insensitive fuzzy check.
func FuzzyMatch(target, input string) bool {
	if input == "" {
		return true
	}
	target = strings.ToLower(target)
	input = strings.ToLower(input)

	inputIdx := 0
	for targetIdx := 0; targetIdx < len(target); targetIdx++ {
		if target[targetIdx] == input[inputIdx] {
			inputIdx++
		}
		if inputIdx == len(input) {
			return true
		}
	}
	return false
}
