//go:build gui

package main

import (
	"context"
	"flag"
	"fmt"
	"image/color"
	"log"
	"os"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/frontend"
)

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
		w := app.NewWindow(
			app.Title("Alloy Wayland"),
			app.Size(800, 600),
		)
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
	)
	list.Axis = layout.Vertical

	// Refresh UI on incoming messages
	client.OnMessage(func(msg api.Message) {
		w.Invalidate()
	})

	for {
		event := w.NextEvent()
		switch e := event.(type) {
		case system.DestroyEvent:
			return e.Err
		case system.FrameEvent:
			gtx := layout.NewContext(&ops, e)

			if sendButton.Clicked(gtx) {
				content := input.Text()
				if content != "" {
					parts := strings.SplitN(content, " ", 3)
					if len(parts) >= 2 {
						target := parts[0]
						method := parts[1]
						payload := ""
						if len(parts) == 3 {
							payload = parts[2]
						}
						
						go func() {
							ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
							defer cancel()
							_, _ = client.Send(ctx, target, method, []byte(payload))
							w.Invalidate()
						}()
						input.SetText("")
					}
				}
			}

			layout.Flex{
				Axis: layout.Vertical,
			}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						title := material.H4(th, "Alloy Core Monitor")
						title.Color = color.NRGBA{R: 0x44, G: 0x88, B: 0xff, A: 0xff}
						return title.Layout(gtx)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					msgs := client.Messages()
					return material.List(th, &list).Layout(gtx, len(msgs), func(gtx layout.Context, i int) layout.Dimensions {
						msg := msgs[i]
						return layout.UniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return material.Body1(th, fmt.Sprintf("[%s] %s: %s", 
								time.Unix(msg.Timestamp, 0).Format(time.Kitchen), 
								msg.Sender, string(msg.Payload))).Layout(gtx)
						})
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
			)
			e.Frame(gtx.Ops)
		}
	}
}
