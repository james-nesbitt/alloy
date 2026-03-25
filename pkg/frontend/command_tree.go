package frontend

import (
	"sort"
	"strings"

	"github.com/james-nesbitt/alloy/api"
)

type ParamInfo struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"` // string, int, bool
	Required bool     `json:"required"`
	Choices  []string `json:"choices,omitempty"` // For enum-like selection
}

type CommandNode struct {
	Key         string                  `json:"key"`
	Description string                  `json:"description"`
	Target      string                  `json:"target"`
	Method      string                  `json:"method"`
	Children    map[string]*CommandNode `json:"children,omitempty"`
	Annotation  string                  `json:"annotation,omitempty"`
	Shortcut    string                  `json:"shortcut,omitempty"`
	Params      []ParamInfo             `json:"params,omitempty"` // Structured parameters
}

func parseParams(raw string) []ParamInfo {
	var params []ParamInfo
	if raw == "" {
		return params
	}
	parts := strings.Split(raw, ",")
	for _, p := range parts {
		if p == "" {
			continue
		}

		info := ParamInfo{Type: "string", Required: false}
		subparts := strings.Split(p, ":")
		info.Name = subparts[0]

		for i := 1; i < len(subparts); i++ {
			switch subparts[i] {
			case "string", "int", "bool":
				info.Type = subparts[i]
			case "required":
				info.Required = true
			default:
				if strings.HasPrefix(subparts[i], "enum(") && strings.HasSuffix(subparts[i], ")") {
					info.Type = "enum"
					choices := subparts[i][5 : len(subparts[i])-1]
					info.Choices = strings.Split(choices, "|")
				}
			}
		}
		params = append(params, info)
	}
	return params
}

// BuildCommandTree constructs a tree from a list of plugin registrations.
func BuildCommandTree(regs []api.Registration) *CommandNode {
	root := &CommandNode{Children: make(map[string]*CommandNode)}

	for _, r := range regs {
		for _, cap := range r.Capabilities {
			// 1. Add to shortcut tree if shortcut exists
			if cap.Shortcut != "" {
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
						if pStr, ok := cap.Annotations["params"]; ok {
							curr.Params = parseParams(pStr)
						}
					}
				}
			}

			// 2. Always add to the method-based lookup for fuzzy search (accessible via ':')
			if _, ok := root.Children[r.ID]; !ok {
				root.Children[r.ID] = &CommandNode{
					Key:      r.ID,
					Children: make(map[string]*CommandNode),
				}
			}
			targetNode := root.Children[r.ID]

			// Use the part after ':' if it's there (e.g. project:open -> open)
			methodKey := cap.Method
			if idx := strings.Index(methodKey, ":"); idx != -1 {
				methodKey = methodKey[idx+1:]
			}

			if _, ok := targetNode.Children[methodKey]; !ok {
				targetNode.Children[methodKey] = &CommandNode{
					Key:         methodKey,
					Description: cap.Description,
					Target:      r.ID,
					Method:      cap.Method,
					Shortcut:    cap.Shortcut,
					Children:    make(map[string]*CommandNode),
				}
				if group, ok := cap.Annotations["group"]; ok {
					targetNode.Children[methodKey].Annotation = group
				}
				if pStr, ok := cap.Annotations["params"]; ok {
					targetNode.Children[methodKey].Params = parseParams(pStr)
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
	FullTitle   string      `json:"full_title"` // e.g., "project open"
	Description string      `json:"description"`
	Shortcut    string      `json:"shortcut,omitempty"`
	Target      string      `json:"target"`
	Method      string      `json:"method"`
	Weight      int         `json:"weight"`           // Fuzzy match score
	Frequency   int         `json:"frequency"`        // Usage count
	Recency     int         `json:"recency"`          // Last use timestamp (offset)
	Status      string      `json:"status"`           // "running", "crashed", etc.
	Group       string      `json:"group,omitempty"`  // For visual categorization
	Params      []ParamInfo `json:"params,omitempty"` // Structured parameters
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
			group := child.Annotation
			if group == "" {
				group = child.Target
			}
			results = append(results, SearchItem{
				FullTitle:   newPrefix,
				Description: child.Description,
				Shortcut:    child.Shortcut,
				Target:      child.Target,
				Method:      child.Method,
				Group:       group,
				Params:      child.Params,
			})
		}

		results = append(results, child.Flatten(newPrefix)...)
	}
	return results
}

// SortItems orders search results by weight (descending) and then lexicographically.
func SortItems(items []SearchItem) {
	sort.Slice(items, func(i, j int) bool {
		// 1. Weight (Fuzzy Score)
		if items[i].Weight != items[j].Weight {
			return items[i].Weight > items[j].Weight
		}
		// 2. Frequency (Popularity)
		if items[i].Frequency != items[j].Frequency {
			return items[i].Frequency > items[j].Frequency
		}
		// 3. Recency (Last used)
		if items[i].Recency != items[j].Recency {
			return items[i].Recency > items[j].Recency
		}
		// 4. Alphabetical
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
