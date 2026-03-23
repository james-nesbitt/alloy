//go:build gui

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
	"github.com/james-nesbitt/alloy/pkg/cmdutil"
)

type guiState struct {
	mode            int
	isLeader        bool
	breadcrumbs     []string
	targets         []frontend.Registration
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
	selectedIdx     int
	filtered        []frontend.SearchItem
	dashboardTiles  map[string]frontend.DashboardTile
	tileOrder       []string
}

const (
	ModeNormal = iota
	ModeCommand
	ModeProjectCreate
	ModeAiSwitch
	ModeAiQuery
)

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

type discoveryMsg struct {
	Targets []frontend.Registration `json:"targets"`
}

func main() {
	name := flag.String("name", "alloy-gui", "Frontend instance name")
	actor := flag.String("actor", "", "Actor identity (defaults to name)")
	socket := flag.String("socket", frontend.GetAlloyRuntimeDir()+"/default.sock", "Socket address")
	sf := cmdutil.RegisterSecurityFlags(flag.CommandLine)
	flag.Parse()

	cmdutil.HandleSecurityError(sf.Validate())

	if *actor == "" {
		*actor = *name
	}

	client, err := frontend.NewClientWithActorAndSecurity(*name, *actor, *socket, *sf.Insecure, *sf.SecurityDir)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		os.Exit(1)
	}

	go func() {
		w := new(app.Window)
		w.Option(app.Title("Alloy Wayland"))
		w.Option(app.Size(800, 600))
		if err := run(w, client); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window, client *frontend.Client) error {
	th := material.NewTheme()
	var ops op.Ops

	var (
		input        widget.Editor
		sendButton   widget.Clickable
		list         widget.List
		projList     widget.List
		projClicks   []widget.Clickable
		wsList       widget.List
		wsClicks     []widget.Clickable
		gui          guiState
	)
	list.Axis = layout.Vertical
	projList.Axis = layout.Vertical
	wsList.Axis = layout.Vertical
	gui.subscriptions = make(map[string]bool)
	gui.recency = make(map[string]int)
	gui.dashboardTiles = make(map[string]frontend.DashboardTile)
	// Initial mock tiles to verify layout
	gui.dashboardTiles["team"] = frontend.DashboardTile{
		Title:   "Team Presence",
		Content: []string{"● You (Online)", "○ James (Away)", "● AI Worker (Idle)"},
		Status:  "Active",
		Actions: []string{"Invite", "Call"},
	}
	gui.dashboardTiles["ai"] = frontend.DashboardTile{
		Title:   "AI Assistant",
		Content: []string{"Last Task: refactor TUI layout", "Status: Ready"},
		Actions: []string{"Ask", "Reset Context"},
	}
	gui.dashboardTiles["project"] = frontend.DashboardTile{
		Title:   "Current Project",
		Content: []string{"Phase: 5 (Team Collaboration)", "Branch: feature/phase-5-dashboards", "Health: Stable"},
		Status:  "OK",
	}
	gui.tileOrder = []string{"team", "ai", "project"}

	// Discovery Loop
	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			resp, err := client.Send(ctx, "command-manager", "discover", nil)
			cancel()
			if err == nil {
				var dMsg discoveryMsg
				if err := json.Unmarshal(resp.Payload, &dMsg); err == nil {
					gui.targets = dMsg.Targets
					gui.commandTree = frontend.BuildCommandTree(gui.targets)

					// Auto-subscribe to events and fetch active project
					for _, t := range gui.targets {
						if t.ID == "events" {
							if !gui.subscriptions["project:opened"] {
								subCtx, subCancel := context.WithTimeout(context.Background(), time.Second)
								subReq, _ := json.Marshal(map[string]string{"topic": "project:opened"})
								_, _ = client.Send(subCtx, "events", "subscribe", subReq)
								subReq3, _ := json.Marshal(map[string]string{"topic": "workspace:set"})
								_, _ = client.Send(subCtx, "events", "subscribe", subReq3)
								subCancel()
								gui.subscriptions["project:opened"] = true
							}
							if !gui.subscriptions["plugin:crashed"] {
								subCtx, subCancel := context.WithTimeout(context.Background(), time.Second)
								subReq, _ := json.Marshal(map[string]string{"topic": "plugin:crashed"})
								_, _ = client.Send(subCtx, "events", "subscribe", subReq)
								subReq2, _ := json.Marshal(map[string]string{"topic": "plugin:load_failed"})
								_, _ = client.Send(subCtx, "events", "subscribe", subReq2)
								subCancel()
								gui.subscriptions["plugin:crashed"] = true
							}
						}
						if t.ID == "project" {
							if gui.activeProject == nil {
								go func() {
									pCtx, pCancel := context.WithTimeout(context.Background(), time.Second)
									defer pCancel()
									pResp, err := client.Send(pCtx, "project", "active", nil)
									if err == nil && pResp.ID != "" {
										var p frontend.Project
										if err := json.Unmarshal(pResp.Payload, &p); err == nil {
											gui.activeProject = &p
											w.Invalidate()
										}
									}
								}()
							}
							// Always refresh the project list
							go func() {
								pCtx, pCancel := context.WithTimeout(context.Background(), time.Second)
								defer pCancel()
								pResp, err := client.Send(pCtx, "project", "list", nil)
								if err == nil {
									var resp struct {
										Projects []frontend.Project `json:"projects"`
									}
									if err := json.Unmarshal(pResp.Payload, &resp); err == nil {
										gui.projects = resp.Projects
										w.Invalidate()
									}
								}
							}()

							// Fetch workspaces
							go func() {
								wCtx, wCancel := context.WithTimeout(context.Background(), time.Second)
								defer wCancel()
								wResp, err := client.Send(wCtx, "project", "list-workspaces", nil)
								if err == nil {
									var wsList []frontend.Workspace
									if err := json.Unmarshal(wResp.Payload, &wsList); err == nil {
										gui.workspaces = wsList
										w.Invalidate()
									}
								}
							}()

							// Fetch active workspace
							go func() {
								wCtx, wCancel := context.WithTimeout(context.Background(), time.Second)
								defer wCancel()
								wResp, err := client.Send(wCtx, "project", "get-active-workspace", nil)
								if err == nil && wResp.ID != "" {
									var ws frontend.Workspace
									if err := json.Unmarshal(wResp.Payload, &ws); err == nil {
										gui.activeWorkspace = &ws
										w.Invalidate()
									}
								}
							}()
						}

						// Heartbeat
						pID := ""
						if gui.activeProject != nil { pID = gui.activeProject.ID }
						payload, _ := json.Marshal(map[string]any{
							"topic": "presence:heartbeat",
							"data": frontend.Presence{
								User:      client.Actor(),
								Status:    "online",
								Client:    "gui",
								LastSeen:  time.Now().Unix(),
								ProjectID: pID,
							},
						})
						go client.Send(context.Background(), "events", "publish", payload)
					}
					w.Invalidate()
				}
			}
			time.Sleep(5 * time.Second)
		}
	}()

	// Refresh UI on incoming messages
	client.OnMessage(func(msg api.Message) {
		if msg.Sender == "events" {
			var ev struct {
				Topic string          `json:"topic"`
				Data  json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &ev); err == nil {
				switch ev.Topic {
				case "project:opened":
					var p frontend.Project
					if err := json.Unmarshal(ev.Data, &p); err == nil {
						gui.activeProject = &p
					}
				case "workspace:set":
					var ws frontend.Workspace
					if err := json.Unmarshal(ev.Data, &ws); err == nil {
						gui.activeWorkspace = &ws
					}
				}
			}
		}

		if msg.Method == "dashboard-update" {
			var tile frontend.DashboardTile
			if err := json.Unmarshal(msg.Payload, &tile); err == nil {
				if gui.dashboardTiles == nil {
					gui.dashboardTiles = make(map[string]frontend.DashboardTile)
				}
				gui.dashboardTiles[msg.Sender] = tile

				found := false
				for _, id := range gui.tileOrder {
					if id == msg.Sender {
						found = true
						break
					}
				}
				if !found {
					gui.tileOrder = append(gui.tileOrder, msg.Sender)
				}
			}
		}
		w.Invalidate()
	})

	for {
		eventEv := w.Event()
		switch e := eventEv.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			// Declare interest in events

			// Initial focus at startup
			if gui.mode == ModeNormal && !gtx.Focused(&gui) && !gtx.Focused(&input) {
				gtx.Execute(key.FocusCmd{Tag: &gui})
			}

			if gui.mode == ModeProjectCreate && !gtx.Focused(&gui.projectCreate.name) && !gtx.Focused(&gui.projectCreate.description) && !gtx.Focused(&gui.projectCreate.submit) && !gtx.Focused(&gui.projectCreate.cancel) {
				gtx.Execute(key.FocusCmd{Tag: &gui.projectCreate.name})
			}

			if gui.mode == ModeAiSwitch && !gtx.Focused(&gui.aiSwitch.providerType) && !gtx.Focused(&gui.aiSwitch.model) && !gtx.Focused(&gui.aiSwitch.url) && !gtx.Focused(&gui.aiSwitch.submit) && !gtx.Focused(&gui.aiSwitch.cancel) {
				gtx.Execute(key.FocusCmd{Tag: &gui.aiSwitch.providerType})
			}

			if gui.mode == ModeAiQuery && !gtx.Focused(&gui.aiQuery.prompt) && !gtx.Focused(&gui.aiQuery.submit) && !gtx.Focused(&gui.aiQuery.cancel) {
				gtx.Execute(key.FocusCmd{Tag: &gui.aiQuery.prompt})
			}

			// Capture keyboard events
			for {
				ev, ok := gtx.Event(
					key.Filter{Focus: &gui},
					key.Filter{Focus: &input},
				)
				if !ok {
					break
				}
				if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
					switch ke.Name {
					case key.NameEscape:
						gui.mode = ModeNormal
						gui.isLeader = false
						gui.breadcrumbs = nil
						gui.selectedIdx = 0
						gtx.Execute(key.FocusCmd{Tag: &gui})
					case ":":
						if gui.mode == ModeNormal {
							gui.mode = ModeCommand
							gui.isLeader = false
							gui.breadcrumbs = nil
							input.SetText(":")
							gui.selectedIdx = 0
							gtx.Execute(key.FocusCmd{Tag: &input})
							goto skipInput
						}
					case key.NameSpace, " ":
						if gui.mode == ModeNormal {
							gui.mode = ModeCommand
							gui.isLeader = true
							gui.breadcrumbs = nil
							gui.selectedIdx = 0
							gtx.Execute(key.FocusCmd{Tag: &input})
							goto skipInput
						}
					case key.NameDownArrow:
						if len(gui.filtered) > 0 {
							gui.selectedIdx = (gui.selectedIdx + 1) % len(gui.filtered)
						}
					case key.NameUpArrow:
						if len(gui.filtered) > 0 {
							gui.selectedIdx = (gui.selectedIdx - 1 + len(gui.filtered)) % len(gui.filtered)
						}
					case key.NameDeleteBackward:
						if input.Text() == "" && gui.isLeader {
							if len(gui.breadcrumbs) > 0 {
								gui.breadcrumbs = gui.breadcrumbs[:len(gui.breadcrumbs)-1]
								gui.selectedIdx = 0
							} else {
								gui.isLeader = false
								input.SetText(":")
								gui.selectedIdx = 0
							}
							gtx.Execute(key.FocusCmd{Tag: &input})
							goto skipInput
						}
					case key.NameReturn:
						if gui.mode == ModeCommand {
							if len(gui.filtered) > 0 && gui.selectedIdx >= 0 && gui.selectedIdx < len(gui.filtered) {
								item := gui.filtered[gui.selectedIdx]
								executeCommand(client, &gui, fmt.Sprintf("%s %s", item.Target, item.Method), w)
								input.SetText("")
								gui.mode = ModeNormal
								gui.isLeader = false
								gui.breadcrumbs = nil
								gui.selectedIdx = 0
								gtx.Execute(key.FocusCmd{Tag: &gui})
							} else {
								content := input.Text()
								if content != "" {
									executeCommand(client, &gui, content, w)
									input.SetText("")
									gui.mode = ModeNormal
									gui.isLeader = false
									gui.breadcrumbs = nil
									gui.selectedIdx = 0
									gtx.Execute(key.FocusCmd{Tag: &gui})
								}
							}
						}
					default:
						// Handle leader sequence (runes only if input is empty)
						nameStr := string(ke.Name)
						if gui.isLeader && input.Text() == "" {
							if nameStr == ":" {
								gui.isLeader = false
								gui.breadcrumbs = nil
								input.SetText(":")
								gui.selectedIdx = 0
								gtx.Execute(key.FocusCmd{Tag: &input})
								goto skipInput
							}

							if len(nameStr) == 1 {
								keyLow := strings.ToLower(nameStr)
								nodeAt := gui.commandTree.Find(gui.breadcrumbs)
								if nodeAt != nil {
									if child, ok := nodeAt.Children[keyLow]; ok {
										if len(child.Children) == 0 {
											executeCommand(client, &gui, fmt.Sprintf("%s %s", child.Target, child.Method), w)
											gui.mode = ModeNormal
											gui.isLeader = false
											gui.breadcrumbs = nil
											input.SetText("")
											gui.selectedIdx = 0
											gtx.Execute(key.FocusCmd{Tag: &gui})
										} else {
											gui.breadcrumbs = append(gui.breadcrumbs, keyLow)
											gui.selectedIdx = 0
										}
										goto skipInput
									}
								}
							}
						}
					}
				}
			}

		skipInput:
			if sendButton.Clicked(gtx) {
				content := input.Text()
				if content != "" {
					executeCommand(client, &gui, content, w)
					input.SetText("")
				}
			}

			// Ensure we have enough clickables
			if len(projClicks) < len(gui.projects) {
				for i := len(projClicks); i < len(gui.projects); i++ {
					projClicks = append(projClicks, widget.Clickable{})
				}
			}
			if len(wsClicks) < len(gui.workspaces) {
				for i := len(wsClicks); i < len(gui.workspaces); i++ {
					wsClicks = append(wsClicks, widget.Clickable{})
				}
			}

			// Handle project clicks
			for i, p := range gui.projects {
				if projClicks[i].Clicked(gtx) {
					executeCommand(client, &gui, "project open "+p.ID, w)
					gui.showProjects = false
				}
			}
			// Handle workspace clicks
			for i, ws := range gui.workspaces {
				if wsClicks[i].Clicked(gtx) {
					executeCommand(client, &gui, "project set-workspace "+ws.ID, w)
					gui.showWorkspaces = false
				}
			}

			layout.Stack{Alignment: layout.S}.Layout(gtx,
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{
						Axis: layout.Vertical,
					}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												title := material.H4(th, "Alloy Core")
												title.Color = color.NRGBA{R: 0x44, G: 0x88, B: 0xff, A: 0xff}
												return title.Layout(gtx)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												wsName := "No Workspace"
												if gui.activeWorkspace != nil {
													wsName = gui.activeWorkspace.Name
												}
												return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													p := material.Body1(th, "WS: "+wsName)
													p.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
													return layout.Stack{}.Layout(gtx,
														layout.Expanded(func(gtx layout.Context) layout.Dimensions {
															return widget.Border{
																Color: color.NRGBA{R: 0x44, G: 0x88, B: 0xff, A: 0xff},
																Width: unit.Dp(1),
															}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																return layout.Dimensions{Size: gtx.Constraints.Min}
															})
														}),
														layout.Stacked(func(gtx layout.Context) layout.Dimensions {
															return layout.UniformInset(unit.Dp(4)).Layout(gtx, p.Layout)
														}),
													)
												})
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												projectName := "No Project"
												if gui.activeProject != nil {
													projectName = gui.activeProject.Name
												}
												return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													p := material.Body1(th, "Project: "+projectName)
													p.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
													return layout.Stack{}.Layout(gtx,
														layout.Expanded(func(gtx layout.Context) layout.Dimensions {
															return widget.Border{
																Color: color.NRGBA{R: 0x44, G: 0xcc, B: 0x44, A: 0xff},
																Width: unit.Dp(1),
															}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																return layout.Dimensions{Size: gtx.Constraints.Min}
															})
														}),
														layout.Stacked(func(gtx layout.Context) layout.Dimensions {
															return layout.UniformInset(unit.Dp(4)).Layout(gtx, p.Layout)
														}),
													)
												})
											}),
										)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										modeStr := "NORMAL"
										modeColor := color.NRGBA{R: 0x44, G: 0x88, B: 0xff, A: 0xff}
										if gui.mode == ModeCommand {
											if gui.isLeader {
												modeStr = "LEADER: " + strings.Join(gui.breadcrumbs, " > ")
												modeColor = color.NRGBA{R: 0xee, G: 0xaa, B: 0x00, A: 0xff}
											} else {
												modeStr = "COMMAND"
												modeColor = color.NRGBA{R: 0xaa, G: 0x00, B: 0xee, A: 0xff}
											}
										}
										d := material.Caption(th, modeStr)
										d.Color = modeColor
										return d.Layout(gtx)
									}),
								)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if !gui.showProjects {
								return layout.Dimensions{}
							}
							return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.List(th, &projList).Layout(gtx, len(gui.projects), func(gtx layout.Context, i int) layout.Dimensions {
									p := gui.projects[i]
									return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return material.Button(th, &projClicks[i], "Project: "+p.Name).Layout(gtx)
									})
								})
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if !gui.showWorkspaces {
								return layout.Dimensions{}
							}
							return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.List(th, &wsList).Layout(gtx, len(gui.workspaces), func(gtx layout.Context, i int) layout.Dimensions {
									ws := gui.workspaces[i]
									return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return material.Button(th, &wsClicks[i], "Workspace: "+ws.Name).Layout(gtx)
									})
								})
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							msgs := client.Messages()
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return drawDashboard(gtx, th, &gui)
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return material.List(th, &list).Layout(gtx, len(msgs), func(gtx layout.Context, i int) layout.Dimensions {
										msg := msgs[i]
										return layout.UniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											formatted := formatMessage(msg)
											label := material.Body1(th, formatted)
											if msg.Method == "plugin:crashed" || msg.Method == "plugin:load_failed" {
												label.Color = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
											}
											return label.Layout(gtx)
										})
									})
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if gui.mode != ModeCommand {
								return layout.Dimensions{}
							}

							content := input.Text()
							if strings.HasPrefix(content, ":") {
								content = content[1:]
							}

							// Update filtered list
							if gui.isLeader && content == "" {
								node := gui.commandTree.Find(gui.breadcrumbs)
								if node != nil {
									gui.filtered = nil
									keys := make([]string, 0, len(node.Children))
									for k := range node.Children {
										keys = append(keys, k)
									}
									sort.Strings(keys)
									for _, k := range keys {
										child := node.Children[k]
										gui.filtered = append(gui.filtered, frontend.SearchItem{
											FullTitle:   k,
											Description: child.Description,
											Target:      child.Target,
											Method:      child.Method,
											Shortcut:    child.Shortcut,
										})
									}
								}
							} else {
								// Fuzzy search
								flattened := gui.commandTree.Flatten("")
								var scored []frontend.SearchItem
								for _, item := range flattened {
									if frontend.FuzzyMatch(item.FullTitle, content) {
										item.Weight = gui.recency[item.Target+" "+item.Method]
										scored = append(scored, item)
									}
								}
								frontend.SortItems(scored)
								if len(scored) > 10 {
									scored = scored[:10]
								}
								gui.filtered = scored
							}

							if gui.selectedIdx >= len(gui.filtered) {
								gui.selectedIdx = 0
							}

							var hints []layout.FlexChild
							if gui.isLeader && content == "" {
								// Title for leader menu
								hints = append(hints, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									txt := "Leader"
									if len(gui.breadcrumbs) > 0 {
										txt += " > " + strings.Join(gui.breadcrumbs, " > ")
									}
									c := material.H6(th, txt)
									c.Color = color.NRGBA{R: 0, G: 200, B: 255, A: 255}
									return layout.UniformInset(unit.Dp(4)).Layout(gtx, c.Layout)
								}))

								// Show grid of children
								var rows []layout.FlexChild
								for i := 0; i < len(gui.filtered); i += 3 {
									end := i + 3
									if end > len(gui.filtered) { end = len(gui.filtered) }
									rowItems := gui.filtered[i:end]
									
									rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										var cols []layout.FlexChild
										for _, item := range rowItems {
											cols = append(cols, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														c := material.Body1(th, item.FullTitle)
														c.Font.Weight = 700
														c.Color = color.NRGBA{R: 255, G: 255, B: 0, A: 255}
														return c.Layout(gtx)
													}),
													layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														desc := item.Description
														if item.Method == "" { desc = "..." } // Branch
														c := material.Caption(th, desc)
														c.Color = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
														return c.Layout(gtx)
													}),
												)
											}))
										}
										return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, cols...)
										})
									}))
								}
								hints = append(hints, rows...)
							} else {
								// Typing or fuzzy mode show vertical list with selection
								for i, item := range gui.filtered {
									isSelected := i == gui.selectedIdx
									hints = append(hints, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Stack{}.Layout(gtx,
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												if !isSelected { return layout.Dimensions{} }
												paint.FillShape(gtx.Ops, color.NRGBA{R: 60, G: 60, B: 60, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															c := material.Caption(th, item.FullTitle)
															c.Font.Weight = 700
															if isSelected { c.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255} }
															return c.Layout(gtx)
														}),
														layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															c := material.Caption(th, item.Description)
															c.Color = color.NRGBA{R: 180, G: 180, B: 180, A: 255}
															if isSelected { c.Color = color.NRGBA{R: 220, G: 220, B: 220, A: 255} }
															return c.Layout(gtx)
														}),
													)
												})
											}),
										)
									}))
								}
							}

							if len(hints) == 0 {
								return layout.Dimensions{}
							}

							return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx, hints...)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										ed := material.Editor(th, &input, "target method payload...")
										return ed.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return material.Button(th, &sendButton, "Send").Layout(gtx)
										})
									}),
								)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							// Status bar at bottom
							return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.Caption(th, "Alloy GUI | F1: help | "+fmt.Sprintf("%d messages", len(client.Messages()))).Layout(gtx)
							})
						}),
					)
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if gui.mode != ModeProjectCreate {
						return layout.Dimensions{}
					}
					// Background dimming
					paintOverlay(gtx, color.NRGBA{A: 150})

					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return widget.Border{
								Color: color.NRGBA{R: 0, G: 0, B: 0, A: 255},
								Width: unit.Dp(2),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Stack{}.Layout(gtx,
									layout.Expanded(func(gtx layout.Context) layout.Dimensions {
										return widget.Border{
											Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255},
											Width: unit.Dp(1),
										}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.Dimensions{Size: gtx.Constraints.Min}
										})
									}),
									layout.Stacked(func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													return material.H6(th, "Create New Project").Layout(gtx)
												}),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														return material.Editor(th, &gui.projectCreate.name, "Name").Layout(gtx)
													})
												}),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														return material.Editor(th, &gui.projectCreate.description, "Description").Layout(gtx)
													})
												}),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEvenly}.Layout(gtx,
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															if gui.projectCreate.submit.Clicked(gtx) {
																name := gui.projectCreate.name.Text()
																description := gui.projectCreate.description.Text()
																if name != "" {
																	payload, _ := json.Marshal(map[string]string{
																		"name":        name,
																		"description": description,
																	})
																	go client.Send(context.Background(), "project", "create", payload)
																	gui.mode = ModeNormal
																	gui.projectCreate.name.SetText("")
																	gui.projectCreate.description.SetText("")
																}
															}
															return material.Button(th, &gui.projectCreate.submit, "Create").Layout(gtx)
														}),
														layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															if gui.projectCreate.cancel.Clicked(gtx) {
																gui.mode = ModeNormal
															}
															return material.Button(th, &gui.projectCreate.cancel, "Cancel").Layout(gtx)
														}),
													)
												}),
											)
										})
									}),
								)
							})
						})
					})
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if gui.mode != ModeAiSwitch {
						return layout.Dimensions{}
					}
					paintOverlay(gtx, color.NRGBA{A: 150})
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return widget.Border{Color: color.NRGBA{A: 255}, Width: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(material.H6(th, "Switch AI Provider").Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return material.Editor(th, &gui.aiSwitch.providerType, "Type (ollama|openai|anthropic)").Layout(gtx)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return material.Editor(th, &gui.aiSwitch.model, "Model").Layout(gtx)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return material.Editor(th, &gui.aiSwitch.url, "URL (e.g. for ollama)").Layout(gtx)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if gui.aiSwitch.submit.Clicked(gtx) {
												t := gui.aiSwitch.providerType.Text()
												m := gui.aiSwitch.model.Text()
												u := gui.aiSwitch.url.Text()
												payload, _ := json.Marshal(map[string]string{"type": t, "model": m, "url": u})
												go client.Send(context.Background(), "ai", "provider:set", payload)
												gui.mode = ModeNormal
											}
											return material.Button(th, &gui.aiSwitch.submit, "Switch").Layout(gtx)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if gui.aiSwitch.cancel.Clicked(gtx) { gui.mode = ModeNormal }
											return material.Button(th, &gui.aiSwitch.cancel, "Cancel").Layout(gtx)
										}),
									)
								})
							})
						})
					})
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if gui.mode != ModeAiQuery {
						return layout.Dimensions{}
					}
					paintOverlay(gtx, color.NRGBA{A: 150})
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return widget.Border{Color: color.NRGBA{A: 255}, Width: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(material.H6(th, "AI Query").Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return material.Editor(th, &gui.aiQuery.prompt, "Ask the AI...").Layout(gtx)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if gui.aiQuery.submit.Clicked(gtx) {
												p := gui.aiQuery.prompt.Text()
												payload, _ := json.Marshal(map[string]string{"prompt": p})
												go client.Send(context.Background(), "ai", "query", payload)
												gui.mode = ModeNormal
											}
											return material.Button(th, &gui.aiQuery.submit, "Ask").Layout(gtx)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if gui.aiQuery.cancel.Clicked(gtx) { gui.mode = ModeNormal }
											return material.Button(th, &gui.aiQuery.cancel, "Cancel").Layout(gtx)
										}),
									)
								})
							})
						})
					})
				}),
			)
			e.Frame(gtx.Ops)
		}
	}
}

func paintOverlay(gtx layout.Context, c color.NRGBA) {
	paint.FillShape(gtx.Ops, c, clip.Rect{Max: gtx.Constraints.Max}.Op())
}

func formatMessage(msg api.Message) string {
	ts := time.Unix(msg.Timestamp, 0).Format("15:04:05")
	if msg.Sender == "events" {
		switch msg.Method {
		case "plugin:crashed", "plugin:load_failed":
			var ev struct {
				Topic string `json:"topic"`
				Data  struct {
					ID    string `json:"id"`
					Error string `json:"error"`
				} `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &ev); err == nil {
				return fmt.Sprintf("[%s] !!! %s (%s): %s", ts, strings.ToUpper(msg.Method[7:]), ev.Data.ID, ev.Data.Error)
			}
		case "chat:message":
			var chatMsg struct {
				Sender  string `json:"sender"`
				Channel string `json:"channel"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(msg.Payload, &chatMsg); err == nil {
				return fmt.Sprintf("[%s] #%s <%s> %s", ts, chatMsg.Channel, chatMsg.Sender, chatMsg.Content)
			}
		case "project:opened":
			var ev struct {
				Topic string           `json:"topic"`
				Data  frontend.Project `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &ev); err == nil {
				return fmt.Sprintf("[%s] Project opened: %s", ts, ev.Data.Name)
			}
		}
	}
	return fmt.Sprintf("[%s] %s: %s", ts, msg.Sender, string(msg.Payload))
}

func drawDashboard(gtx layout.Context, th *material.Theme, gui *guiState) layout.Dimensions {
	if len(gui.tileOrder) == 0 {
		return layout.Dimensions{}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			title := material.H6(th, "Project Dashboard")
			title.Color = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, title.Layout)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var rows []layout.FlexChild
			for i := 0; i < len(gui.tileOrder); i += 2 {
				end := i + 2
				if end > len(gui.tileOrder) {
					end = len(gui.tileOrder)
				}
				rowTiles := gui.tileOrder[i:end]

				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					var cols []layout.FlexChild
					for _, id := range rowTiles {
						tile := gui.dashboardTiles[id]
						cols = append(cols, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return widget.Border{
									Color: color.NRGBA{R: 100, G: 100, B: 100, A: 255},
									Width: unit.Dp(1),
									CornerRadius: unit.Dp(4),
								}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
													layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
														t := material.Body1(th, tile.Title)
														t.Font.Weight = 700
														return t.Layout(gtx)
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														s := material.Caption(th, tile.Status)
														s.Color = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
														return s.Layout(gtx)
													}),
												)
											}),
											layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												content := strings.Join(tile.Content, "\n")
												return material.Caption(th, content).Layout(gtx)
											}),
										)
									})
								})
							})
						}))
					}
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, cols...)
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		}),
	)
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
		
		if gui.recency == nil { gui.recency = make(map[string]int) }
		gui.recency[target+" "+method] = int(time.Now().Unix())

		payload := ""
		if len(parts) > 2 {
			payload = strings.Join(parts[2:], " ")
		}

		// Specialized handlers
		if target == "project" && method == "open" && payload == "" {
			gui.showProjects = !gui.showProjects
			gui.showWorkspaces = false
			w.Invalidate()
			return
		}

		if target == "project" && method == "list-workspaces" && payload == "" {
			gui.showWorkspaces = !gui.showWorkspaces
			gui.showProjects = false
			w.Invalidate()
			return
		}

		if (target == "project") && (method == "set-workspace") && payload == "" {
			gui.showWorkspaces = !gui.showWorkspaces
			gui.showProjects = false
			w.Invalidate()
			return
		}

		if target == "project" && method == "create" && payload == "" {
			gui.mode = ModeProjectCreate
			w.Invalidate()
			return
		}

		if (target == "ai") && (method == "switch" || method == "provider:set") && payload == "" {
			gui.mode = ModeAiSwitch
			w.Invalidate()
			return
		}

		if (target == "ai") && method == "query" && payload == "" {
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
