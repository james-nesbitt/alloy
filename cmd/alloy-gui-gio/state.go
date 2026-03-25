package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

type formState struct {
	title   string
	params  []frontend.ParamInfo
	editors []widget.Editor
	errors  []string
	submit  widget.Clickable
	cancel  widget.Clickable
}

type guiState struct {
	mode            int
	isLeader        bool
	breadcrumbs     []string
	targets         []api.Registration
	commandTree     *frontend.CommandNode
	recency         map[string]int
	activeProject   *frontend.Project
	activeWorkspace *frontend.Workspace
	projects        []frontend.Project
	workspaces      []frontend.Workspace
	showProjects    bool
	showWorkspaces  bool
	subscriptions   map[string]bool
	form            formState
	aiSwitch        aiSwitchState
	aiQuery         aiQueryState
	menuBtn         widget.Clickable
	selectedIdx     int
	filtered        []frontend.SearchItem
	dashboardTiles  map[string]frontend.DashboardTile
	tileOrder       []string
	frequencies     map[string]int
}

type aiSwitchState struct {
	providerType widget.Editor
	model        widget.Editor
	url          widget.Editor
	submit       widget.Clickable
	cancel       widget.Clickable
}

type aiQueryState struct {
	prompt widget.Editor
	submit widget.Clickable
	cancel widget.Clickable
}

func (g *guiState) handleEvent(ev key.Event, input *widget.Editor, client *frontend.Client, w *app.Window, gtx layout.Context) {
	if ev.State != key.Press {
		return
	}

	switch ev.Name {
	case key.NameF1:
		if g.mode == ModeCommand {
			g.mode = ModeNormal
		} else {
			g.mode = ModeCommand
			g.isLeader = false
			input.SetText(":")
		}
	case key.NameEscape:
		g.mode = ModeNormal
		g.isLeader = false
		g.breadcrumbs = nil
		g.selectedIdx = 0
		if gtx.Ops != nil {
			gtx.Execute(key.FocusCmd{Tag: g})
		}
	case ":", "·":
		if g.mode == ModeNormal {
			g.mode = ModeCommand
			g.isLeader = false
			g.breadcrumbs = nil
			input.SetText(":")
			g.selectedIdx = 0
			if gtx.Ops != nil {
				gtx.Execute(key.FocusCmd{Tag: input})
			}
		}
	case key.NameSpace, " ":
		if g.mode == ModeNormal {
			g.mode = ModeCommand
			g.isLeader = true
			g.breadcrumbs = nil
			g.selectedIdx = 0
			if gtx.Ops != nil {
				gtx.Execute(key.FocusCmd{Tag: input})
			}
		}
	case key.NameDownArrow:
		if len(g.filtered) > 0 {
			g.selectedIdx = (g.selectedIdx + 1) % len(g.filtered)
		}
	case key.NameUpArrow:
		if len(g.filtered) > 0 {
			g.selectedIdx = (g.selectedIdx - 1 + len(g.filtered)) % len(g.filtered)
		}
	case key.NameDeleteBackward:
		if input.Text() == "" && g.isLeader {
			if len(g.breadcrumbs) > 0 {
				g.breadcrumbs = g.breadcrumbs[:len(g.breadcrumbs)-1]
				g.selectedIdx = 0
			} else {
				g.isLeader = false
				input.SetText(":")
				g.selectedIdx = 0
			}
			if gtx.Ops != nil {
				gtx.Execute(key.FocusCmd{Tag: input})
			}
		}
	case key.NameReturn:
		if g.mode == ModeCommand {
			if len(g.filtered) > 0 && g.selectedIdx >= 0 && g.selectedIdx < len(g.filtered) {
				item := g.filtered[g.selectedIdx]
				if len(item.Params) > 0 {
					g.form.title = item.Target + " " + item.Method
					g.form.params = item.Params
					g.form.editors = make([]widget.Editor, len(item.Params))
					g.mode = ModeForm
					w.Invalidate()
					return
				}
				executeCommand(client, g, fmt.Sprintf("%s %s", item.Target, item.Method), w)
				input.SetText("")
				g.mode = ModeNormal
				g.isLeader = false
				g.breadcrumbs = nil
				g.selectedIdx = 0
				if gtx.Ops != nil {
					gtx.Execute(key.FocusCmd{Tag: g})
				}
			} else {
				content := input.Text()
				if content != "" {
					executeCommand(client, g, content, w)
					input.SetText("")
					g.mode = ModeNormal
					g.isLeader = false
					g.breadcrumbs = nil
					g.selectedIdx = 0
					if gtx.Ops != nil {
						gtx.Execute(key.FocusCmd{Tag: g})
					}
				}
			}
		}
	default:
		// Handle leader sequence (runes only if input is empty)
		nameStr := string(ev.Name)
		if g.isLeader && input.Text() == "" {
			if nameStr == ":" {
				g.isLeader = false
				g.breadcrumbs = nil
				input.SetText(":")
				g.selectedIdx = 0
				if gtx.Ops != nil {
					gtx.Execute(key.FocusCmd{Tag: input})
				}
				return
			}

			if len(nameStr) == 1 {
				keyLow := strings.ToLower(nameStr)
				nodeAt := g.commandTree.Find(g.breadcrumbs)
				if nodeAt != nil {
					if child, ok := nodeAt.Children[keyLow]; ok {
						if len(child.Children) == 0 {
							if len(child.Params) > 0 {
								g.form.title = child.Target + " " + child.Method
								g.form.params = child.Params
								g.form.editors = make([]widget.Editor, len(child.Params))
								g.mode = ModeForm
								w.Invalidate()
								return
							}

							executeCommand(client, g, fmt.Sprintf("%s %s", child.Target, child.Method), w)
							g.mode = ModeNormal
							g.isLeader = false
							g.breadcrumbs = nil
							input.SetText("")
							g.selectedIdx = 0
							if gtx.Ops != nil {
								gtx.Execute(key.FocusCmd{Tag: g})
							}
						} else {
							g.breadcrumbs = append(g.breadcrumbs, keyLow)
							g.selectedIdx = 0
						}
						return
					}
				}
			}
		}
	}
}

func (g *guiState) updateFiltered(content string) {
	if strings.HasPrefix(content, ":") {
		content = content[1:]
	}

	if g.isLeader && content == "" {
		node := g.commandTree.Find(g.breadcrumbs)
		if node != nil {
			g.filtered = nil
			keys := make([]string, 0, len(node.Children))
			for k := range node.Children {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				child := node.Children[k]
				g.filtered = append(g.filtered, frontend.SearchItem{
					FullTitle:   k,
					Description: child.Description,
					Target:      child.Target,
					Method:      child.Method,
					Shortcut:    child.Shortcut,
					Group:       child.Annotation,
				})
			}
		}
	} else {
		flattened := g.commandTree.Flatten("")
		var scored []frontend.SearchItem
		for _, item := range flattened {
			score := frontend.FuzzyScore(item.FullTitle, content)
			if score > 0 {
				item.Weight = score

				// Frequency & Recency boost
				key := item.Target + " " + item.Method
				if freq, ok := g.frequencies[key]; ok {
					item.Frequency = freq
				}
				if lastUsed, ok := g.recency[key]; ok {
					item.Recency = lastUsed
				}

				// Contextual boost
				if (g.mode == ModeAiSwitch || g.mode == ModeAiQuery) && (item.Target == "ai" || item.Target == "chat") {
					item.Weight += 200
				}
				if (g.activeProject != nil) && (item.Target == "project") {
					item.Weight += 50
				}

				scored = append(scored, item)
			}
		}
		frontend.SortItems(scored)
		if len(scored) > 10 {
			scored = scored[:10]
		}
		g.filtered = scored
	}

	if g.selectedIdx >= len(g.filtered) {
		g.selectedIdx = 0
	}
}

func executeCommand(client *frontend.Client, gui *guiState, content string, w *app.Window) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, ":") {
		content = content[1:]
	}
	parts := strings.Fields(content)
	if len(parts) >= 2 {
		target := parts[0]
		method := parts[1]
		key := target + " " + method

		if gui.recency == nil {
			gui.recency = make(map[string]int)
		}
		if gui.frequencies == nil {
			gui.frequencies = make(map[string]int)
		}
		gui.recency[key] = int(time.Now().Unix())
		gui.frequencies[key]++

		// Normalized method for internal checks (strip prefix if it matches target)
		cleanMethod := method
		if idx := strings.Index(method, ":"); idx != -1 {
			if method[:idx] == target {
				cleanMethod = method[idx+1:]
			}
		}

		payload := ""
		if len(parts) > 2 {
			payload = strings.Join(parts[2:], " ")
		}

		// Specialized handlers
		if target == "project" && cleanMethod == "open" && payload == "" {
			gui.showProjects = !gui.showProjects
			gui.showWorkspaces = false
			w.Invalidate()
			return
		}

		if (target == "project") && (cleanMethod == "set-workspace" || cleanMethod == "list-workspaces") && payload == "" {
			gui.showWorkspaces = !gui.showWorkspaces
			gui.showProjects = false
			w.Invalidate()
			return
		}

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = client.Send(ctx, target, method, []byte(payload))
			w.Invalidate()
		}()
	}
}
