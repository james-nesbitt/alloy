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
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/frontend"
)

type registration struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Capabilities []api.Capability `json:"capabilities,omitempty"`
}

type discoveryMsg struct {
	Targets []registration `json:"targets"`
}

const (
	ModeNormal = iota
	ModeCommand
)

type guiState struct {
	mode        int
	isLeader    bool
	breadcrumbs []string
	targets     []registration
}

func main() {
	name := flag.String("name", "alloy-gui", "Frontend instance name")
	socket := flag.String("socket", frontend.GetAlloyRuntimeDir()+"/default.sock", "Socket address")
	insecure := flag.Bool("insecure", false, "Disable mTLS")
	flag.Parse()

	client, err := frontend.NewClient(*name, *socket, *insecure)
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
		input      widget.Editor
		sendButton widget.Clickable
		list       widget.List
		gui        guiState
	)
	list.Axis = layout.Vertical

	// Discovery Loop
	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			resp, err := client.Send(ctx, "plugin-command-manager", "discover", nil)
			cancel()
			if err == nil {
				var dMsg discoveryMsg
				if err := json.Unmarshal(resp.Payload, &dMsg); err == nil {
					gui.targets = dMsg.Targets
					w.Invalidate()
				}
			}
			time.Sleep(5 * time.Second)
		}
	}()

	// Refresh UI on incoming messages
	client.OnMessage(func(msg api.Message) {
		w.Invalidate()
	})

	for {
		eventEv := w.Event()
		switch e := eventEv.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			// Declare interest in events for the 'gui' tag
			event.Op(gtx.Ops, &gui)

			// Initial focus at startup
			if gui.mode == ModeNormal && !gtx.Focused(&gui) && !gtx.Focused(&input) {
				gtx.Execute(key.FocusCmd{Tag: &gui})
			}

			// Capture keyboard events
			for {
				ev, ok := gtx.Event(key.Filter{Focus: &gui}, key.Filter{Focus: &input})
				if !ok {
					break
				}
				if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
					switch ke.Name {
					case key.NameEscape:
						gui.mode = ModeNormal
						gui.isLeader = false
						gui.breadcrumbs = nil
						gtx.Execute(key.FocusCmd{Tag: &gui})
					case key.NameReturn:
						if gui.mode == ModeCommand {
							content := input.Text()
							if content != "" {
								executeCommand(client, &gui, content, w)
								input.SetText("")
								gui.mode = ModeNormal
								gui.isLeader = false
								gui.breadcrumbs = nil
								gtx.Execute(key.FocusCmd{Tag: &gui})
							}
						}
					case key.NameSpace, " ":
						if gui.mode == ModeNormal {
							gui.mode = ModeCommand
							gui.isLeader = true
							gui.breadcrumbs = nil
							gtx.Execute(key.FocusCmd{Tag: &input})
						}
					default:
						// Handle leader sequence (runes only if len name is actually short or name is Space)
						nameStr := string(ke.Name)
						if gui.isLeader && len(nameStr) == 1 {
							gui.breadcrumbs = append(gui.breadcrumbs, strings.ToLower(nameStr))
							sequence := strings.Join(gui.breadcrumbs, " ")

							// Check for matches
							for _, target := range gui.targets {
								for _, cap := range target.Capabilities {
									if cap.Shortcut == sequence {
										executeCommand(client, &gui, fmt.Sprintf("%s %s", target.ID, cap.Method), w)
										gui.mode = ModeNormal
										gui.isLeader = false
										gui.breadcrumbs = nil
										input.SetText("")
										gtx.Execute(key.FocusCmd{Tag: &gui})
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

			layout.Flex{
				Axis: layout.Vertical,
			}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								title := material.H4(th, "Alloy Core")
								title.Color = color.NRGBA{R: 0x44, G: 0x88, B: 0xff, A: 0xff}
								return title.Layout(gtx)
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
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					msgs := client.Messages()
					return material.List(th, &list).Layout(gtx, len(msgs), func(gtx layout.Context, i int) layout.Dimensions {
						msg := msgs[i]
						return layout.UniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return material.Body1(th, fmt.Sprintf("[%s] %s: %s", 
								time.Unix(msg.Timestamp, 0).Format("15:04:05"), 
								msg.Sender, string(msg.Payload))).Layout(gtx)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !gui.isLeader {
						return layout.Dimensions{}
					}
					// Show filtered command hints
					prefix := strings.Join(gui.breadcrumbs, " ")
					var hints []string
					for _, t := range gui.targets {
						for _, c := range t.Capabilities {
							if prefix == "" || strings.HasPrefix(c.Shortcut, prefix) {
								reminder := strings.TrimPrefix(c.Shortcut, prefix)
								reminder = strings.TrimSpace(reminder)
								hints = append(hints, fmt.Sprintf("%-2s %s", reminder, c.Method))
							}
						}
					}
					if len(hints) == 0 {
						return layout.Dimensions{}
					}
					return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return material.Caption(th, "Hints: "+strings.Join(hints, " | ")).Layout(gtx)
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
			e.Frame(gtx.Ops)
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
		payload := ""
		if len(parts) > 2 {
			payload = strings.Join(parts[2:], " ")
		}

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = client.Send(ctx, target, method, []byte(payload))
			w.Invalidate()
		}()
	}
}
