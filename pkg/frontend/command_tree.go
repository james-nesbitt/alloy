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
	if n == nil {
		return nil
	}
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
	if n == nil {
		return results
	}

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

// FuzzyScore calculates a numeric match quality (higher is better).
func FuzzyScore(target, input string) int {
	if input == "" {
		return 100 // Base score for no input
	}
	targetLower := strings.ToLower(target)
	inputLower := strings.ToLower(input)

	// 1. Exact match bonus
	if targetLower == inputLower {
		return 1000
	}

	// 2. Prefix match bonus
	if strings.HasPrefix(targetLower, inputLower) {
		return 500 + (len(input) * 10)
	}

	// 3. Subsequence match
	score := 0
	inputIdx := 0
	lastMatchIdx := -1
	consecutiveBonus := 0

	for targetIdx := 0; targetIdx < len(targetLower); targetIdx++ {
		if targetLower[targetIdx] == inputLower[inputIdx] {
			// Basic match point
			score += 10

			// Bonus for consecutive characters
			if lastMatchIdx != -1 && targetIdx == lastMatchIdx+1 {
				consecutiveBonus += 5
				score += consecutiveBonus
			} else {
				consecutiveBonus = 0
			}

			// Bonus for match at word boundary (space, hyphen, underscore)
			if targetIdx == 0 || targetLower[targetIdx-1] == ' ' || targetLower[targetIdx-1] == '-' || targetLower[targetIdx-1] == '_' || targetLower[targetIdx-1] == ':' {
				score += 50
			}

			lastMatchIdx = targetIdx
			inputIdx++
		}
		if inputIdx == len(inputLower) {
			// Found all characters. Penalize for total length gap (prefer shorter matches)
			score -= (len(target) - len(input))
			return score
		}
	}

	return -1 // No match
}

// FuzzyMatch is kept for backward compatibility but calls FuzzyScore.
func FuzzyMatch(target, input string) bool {
	return FuzzyScore(target, input) > 0
}
