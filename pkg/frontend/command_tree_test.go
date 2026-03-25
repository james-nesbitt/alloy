package frontend

import (
	"github.com/james-nesbitt/alloy/api"
	"testing"
)

func TestBuildCommandTree(t *testing.T) {
	regs := []api.Registration{
		{
			ID: "project",
			Capabilities: []api.Capability{
				{Method: "open", Description: "Open project", Shortcut: "p o"},
				{Method: "list", Description: "List projects", Shortcut: "p l"},
			},
		},
		{
			ID: "chat",
			Capabilities: []api.Capability{
				{Method: "send", Description: "Send message", Shortcut: "c s"},
			},
		},
	}

	root := BuildCommandTree(regs)

	// Check shortcut "p o"
	p := root.Children["p"]
	if p == nil {
		t.Fatal("Expected shortcut 'p' node")
	}
	o := p.Children["o"]
	if o == nil {
		t.Fatal("Expected shortcut 'o' node under 'p'")
	}
	if o.Target != "project" || o.Method != "open" {
		t.Errorf("Expected project:open, got %s:%s", o.Target, o.Method)
	}

	// Check method lookup ":project"
	project := root.Children["project"]
	if project == nil {
		t.Fatal("Expected direct method lookup 'project' node")
	}
	open := project.Children["open"]
	if open == nil {
		t.Fatal("Expected 'open' under 'project'")
	}

	// Flattening
	items := root.Flatten("")
	foundPo := false
	foundPl := false
	foundCs := false
	for _, item := range items {
		if item.FullTitle == "p o" {
			foundPo = true
		}
		if item.FullTitle == "p l" {
			foundPl = true
		}
		if item.FullTitle == "c s" {
			foundCs = true
		}
	}
	if !foundPo || !foundPl || !foundCs {
		t.Errorf("Missing expected flattened items: po=%v, pl=%v, cs=%v", foundPo, foundPl, foundCs)
	}
}

func TestFuzzyScore(t *testing.T) {
	cases := []struct {
		target string
		input  string
		wantGt int
	}{
		{"project open", "po", 0},
		{"project open", "proj open", 0},
		{"project open", "p o", 0},
		{"project open", "x", -1},
		{"project open", "project open", 999},
	}

	for _, c := range cases {
		got := FuzzyScore(c.target, c.input)
		if c.wantGt == -1 {
			if got != -1 {
				t.Errorf("FuzzyScore(%q, %q) = %d, want -1", c.target, c.input, got)
			}
		} else {
			if got <= c.wantGt {
				t.Errorf("FuzzyScore(%q, %q) = %d, want > %d", c.target, c.input, got, c.wantGt)
			}
		}
	}
}
