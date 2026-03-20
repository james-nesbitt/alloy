package main

import (
	"strings"
)

type CommandNode struct {
	Key         string
	Description string
	Target      string
	Method      string
	Children    map[string]*CommandNode
	Annotation  string
}

func BuildCommandTree(targets []registration) *CommandNode {
	root := &CommandNode{Children: make(map[string]*CommandNode)}

	for _, t := range targets {
		for _, cap := range t.Capabilities {
			if cap.Shortcut == "" {
				continue
			}

			keys := strings.Fields(cap.Shortcut)
			curr := root
			for i, k := range keys {
				if curr.Children[k] == nil {
					curr.Children[k] = &CommandNode{
						Key:      k,
						Children: make(map[string]*CommandNode),
					}
				}
				curr = curr.Children[k]

				// Last key in shortcut
				if i == len(keys)-1 {
					curr.Description = cap.Description
					curr.Target = t.ID
					curr.Method = cap.Method
					if group, ok := cap.Annotations["group"]; ok {
						curr.Annotation = group
					}
				}
			}
		}
	}

	return root
}

func (n *CommandNode) Find(path []string) *CommandNode {
	curr := n
	for _, k := range path {
		if curr.Children[k] == nil {
			return nil
		}
		curr = curr.Children[k]
	}
	return curr
}

func (n *CommandNode) VisibleOptions() []Option {
	var opts []Option
	for k, child := range n.Children {
		opts = append(opts, Option{
			Key:         k,
			Description: child.Description,
			Annotation:  child.Annotation,
			IsDir:       len(child.Children) > 0,
		})
	}
	return opts
}

type Option struct {
	Key         string
	Description string
	Annotation  string
	IsDir       bool
}
