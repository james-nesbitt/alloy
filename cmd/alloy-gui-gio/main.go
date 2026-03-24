//go:build gui

package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/cmdutil"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

const (
	ModeNormal = iota
	ModeCommand
	ModeForm
	ModeAiSwitch
	ModeAiQuery
)

type discoveryMsg struct {
	Targets []api.Registration `json:"targets"`
}

func main() {
	socketAddr := flag.String("socket", frontend.GetAlloyRuntimeDir()+"/default.sock", "Alloy core IPC address")
	actor := flag.String("actor", "alloy-gui", "Actor identity")
	_ = flag.Bool("debug", false, "Enable debug logging")
	sf := cmdutil.RegisterSecurityFlags(flag.CommandLine)
	flag.Parse()

	cmdutil.HandleSecurityError(sf.Validate())

	client, err := frontend.NewClientWithActorAndSecurity("alloy-gui", *actor, *socketAddr, *sf.Insecure, *sf.SecurityDir)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	go func() {
		w := new(app.Window)
		w.Option(app.Title("Alloy Core"))
		if err := run(w, client); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window, client *frontend.Client) error {
	th := material.NewTheme()
	applySystemTheme(th)
	var ops op.Ops

	var (
		input      widget.Editor
		sendButton widget.Clickable
		list       widget.List
		projList   widget.List
		projClicks []widget.Clickable
		wsList     widget.List
		wsClicks   []widget.Clickable
		gui        guiState
	)
	list.Axis = layout.Vertical
	projList.Axis = layout.Vertical
	wsList.Axis = layout.Vertical
	gui.subscriptions = make(map[string]bool)
	gui.recency = make(map[string]int)
	gui.frequencies = make(map[string]int)
	gui.dashboardTiles = make(map[string]frontend.DashboardTile)

	discoverCh := make(chan bool, 1)
	discoverCh <- true

	go func() {
		startupAttempts := 0
		for {
			select {
			case <-discoverCh:
			case <-time.After(2 * time.Second):
				if startupAttempts < 15 {
					startupAttempts++
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			resp, err := client.Send(ctx, "command-manager", "discover", nil)
			cancel()
			if err == nil {
				var dMsg discoveryMsg
				if err := json.Unmarshal(resp.Payload, &dMsg); err == nil && len(dMsg.Targets) > 0 {
					gui.targets = dMsg.Targets
					gui.commandTree = frontend.BuildCommandTree(gui.targets)

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
						}
						if t.ID == "project" {
							if gui.activeProject == nil {
								pCtx, pCancel := context.WithTimeout(context.Background(), time.Second)
								pResp, err := client.Send(pCtx, "project", "get_active", nil)
								pCancel()
								if err == nil && pResp.ID != "" {
									var p frontend.Project
									if err := json.Unmarshal(pResp.Payload, &p); err == nil {
										gui.activeProject = &p
									}
								}
							}
						}
					}
					w.Invalidate()
				}
			}
			time.Sleep(5 * time.Second)
		}
	}()

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

			for {
				ev, ok := gtx.Event(
					key.Filter{Focus: &gui},
					key.Filter{Focus: &input},
				)
				if !ok {
					break
				}
				if ke, ok := ev.(key.Event); ok {
					gui.handleEvent(ke, &input, client, w, gtx)
				}
			}

			if gui.mode == ModeNormal && strings.HasPrefix(input.Text(), ":") {
				gui.mode = ModeCommand
				gui.isLeader = false
			}

			if sendButton.Clicked(gtx) {
				content := input.Text()
				if content != "" {
					executeCommand(client, &gui, content, w)
					input.SetText("")
				}
			}

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

			for i, p := range gui.projects {
				if projClicks[i].Clicked(gtx) {
					executeCommand(client, &gui, "project open "+p.ID, w)
					gui.showProjects = false
				}
			}
			for i, ws := range gui.workspaces {
				if wsClicks[i].Clicked(gtx) {
					executeCommand(client, &gui, "project set-workspace "+ws.ID, w)
					gui.showWorkspaces = false
				}
			}

			layout.Stack{Alignment: layout.Center}.Layout(gtx,
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return renderMainLayout(gtx, th, client, &gui, &input, &sendButton, &list, &projList, projClicks, &wsList, wsClicks)
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return renderModals(gtx, th, client, &gui)
				}),
			)
			e.Frame(gtx.Ops)
		}
	}
}
