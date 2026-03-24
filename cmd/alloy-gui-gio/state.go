package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

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
	projectCreate   projectCreateState
	aiSwitch        aiSwitchState
	aiQuery         aiQueryState
	menuBtn         widget.Clickable
	selectedIdx     int
	filtered        []frontend.SearchItem
	dashboardTiles  map[string]frontend.DashboardTile
	tileOrder       []string
}

type projectCreateState struct {
	name        widget.Editor
	description widget.Editor
	submit      widget.Clickable
	cancel      widget.Clickable
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

func executeCommand(client *frontend.Client, gui *guiState, content string, w *app.Window) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, ":") {
		content = content[1:]
	}
	parts := strings.Fields(content)
	if len(parts) >= 2 {
		target := parts[0]
		method := parts[1]

		// Normalized method for internal checks (strip prefix if it matches target)
		cleanMethod := method
		if idx := strings.Index(method, ":"); idx != -1 {
			if method[:idx] == target {
				cleanMethod = method[idx+1:]
			}
		}

		if gui.recency == nil {
			gui.recency = make(map[string]int)
		}
		gui.recency[target+" "+method] = int(time.Now().Unix())

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

		if target == "project" && cleanMethod == "list-workspaces" && payload == "" {
			gui.showWorkspaces = !gui.showWorkspaces
			gui.showProjects = false
			w.Invalidate()
			return
		}

		if (target == "project") && (cleanMethod == "set-workspace") && payload == "" {
			gui.showWorkspaces = !gui.showWorkspaces
			gui.showProjects = false
			w.Invalidate()
			return
		}

		if target == "project" && cleanMethod == "create" && payload == "" {
			gui.mode = ModeProjectCreate
			w.Invalidate()
			return
		}

		if (target == "ai") && (cleanMethod == "switch" || cleanMethod == "provider:set") && payload == "" {
			gui.mode = ModeAiSwitch
			w.Invalidate()
			return
		}

		if (target == "ai") && cleanMethod == "query" && payload == "" {
			gui.mode = ModeAiQuery
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
