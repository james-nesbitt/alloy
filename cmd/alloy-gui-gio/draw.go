package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

func paintOverlay(gtx layout.Context, c color.NRGBA) {
	paint.FillShape(gtx.Ops, c, clip.Rect{Max: gtx.Constraints.Max}.Op())
}

func formatMessage(msg api.Message) string {
	ts := time.Unix(msg.Timestamp, 0).Format("15:04:05")
	if msg.Sender == "events" {
		var ev struct {
			Topic string          `json:"topic"`
			Data  json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &ev); err == nil {
			switch ev.Topic {
			case "chat:message":
				var chatMsg struct {
					Sender  string `json:"sender"`
					Channel string `json:"channel"`
					Content string `json:"content"`
				}
				if err := json.Unmarshal(ev.Data, &chatMsg); err == nil {
					return fmt.Sprintf("[%s] #%s <%s> %s", ts, chatMsg.Channel, chatMsg.Sender, chatMsg.Content)
				}
			}
			return fmt.Sprintf("[%s] Event: %s", ts, ev.Topic)
		}
	}
	return fmt.Sprintf("[%s] %s: %s", ts, msg.Sender, string(msg.Payload))
}

func renderMainLayout(gtx layout.Context, th *material.Theme, client *frontend.Client, gui *guiState, input *widget.Editor, sendButton *widget.Clickable, list *widget.List, projList *widget.List, projClicks []widget.Clickable, wsList *widget.List, wsClicks []widget.Clickable) layout.Dimensions {
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
									return layout.Stack{}.Layout(gtx,
										layout.Expanded(func(gtx layout.Context) layout.Dimensions {
											paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 120, B: 215, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
											return layout.Dimensions{Size: gtx.Constraints.Min}
										}),
										layout.Stacked(func(gtx layout.Context) layout.Dimensions {
											p := material.Body2(th, "WS: "+wsName)
											p.Font.Weight = 700
											p.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
											return layout.UniformInset(unit.Dp(6)).Layout(gtx, p.Layout)
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
									return layout.Stack{}.Layout(gtx,
										layout.Expanded(func(gtx layout.Context) layout.Dimensions {
											paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 150, B: 0, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
											return layout.Dimensions{Size: gtx.Constraints.Min}
										}),
										layout.Stacked(func(gtx layout.Context) layout.Dimensions {
											p := material.Body2(th, "Project: "+projectName)
											p.Font.Weight = 700
											p.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
											return layout.UniformInset(unit.Dp(6)).Layout(gtx, p.Layout)
										}),
									)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(th, &gui.menuBtn, "MENU (F1)")
									btn.TextSize = unit.Sp(14)
									btn.Background = color.NRGBA{R: 150, G: 0, B: 150, A: 255}
									if gui.menuBtn.Clicked(gtx) {
										if gui.mode == ModeCommand {
											gui.mode = ModeNormal
										} else {
											gui.mode = ModeCommand
											gui.isLeader = false
											input.SetText(":")
										}
									}
									return btn.Layout(gtx)
								})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						modeStr := "NORMAL"
						modeColor := color.NRGBA{R: 0x44, G: 0x88, B: 0xff, A: 0xff}
						if gui.mode == ModeCommand {
							if gui.isLeader {
								modeStr = "LEADER"
								if len(gui.breadcrumbs) > 0 {
									modeStr += ": " + strings.Join(gui.breadcrumbs, " > ")
								}
								modeColor = color.NRGBA{R: 150, G: 50, B: 0, A: 255}
							} else {
								modeStr = "COMMAND"
								modeColor = color.NRGBA{R: 200, G: 0, B: 200, A: 255}
							}
						}
						return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							d := material.Caption(th, modeStr)
							d.Color = modeColor
							return d.Layout(gtx)
						})
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !gui.showProjects {
				return layout.Dimensions{}
			}
			return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.List(th, projList).Layout(gtx, len(gui.projects), func(gtx layout.Context, i int) layout.Dimensions {
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
				return material.List(th, wsList).Layout(gtx, len(gui.workspaces), func(gtx layout.Context, i int) layout.Dimensions {
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
					return drawDashboard(gtx, th, gui)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return material.List(th, list).Layout(gtx, len(msgs), func(gtx layout.Context, i int) layout.Dimensions {
						msg := msgs[i]
						return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							formatted := formatMessage(msg)
							label := material.Body1(th, formatted)
							label.TextSize = unit.Sp(16)
							if msg.Method == "plugin:crashed" || msg.Method == "plugin:load_failed" {
								label.Color = color.NRGBA{R: 255, G: 50, B: 50, A: 255}
							} else {
								label.Color = th.Fg
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
			gui.updateFiltered(content)

			return widget.Border{
				Color: color.NRGBA{A: 255},
				Width: unit.Dp(2),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						bg := th.Bg
						bg.A = 245
						paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Min}.Op())
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						var hints []layout.FlexChild
						if gui.isLeader && content == "" {
							hints = append(hints, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								txt := "LEADER MODE"
								if len(gui.breadcrumbs) > 0 {
									txt += " > " + strings.Join(gui.breadcrumbs, " > ")
								}
								c := material.H6(th, txt)
								c.Color = color.NRGBA{R: 255, G: 200, B: 0, A: 255}
								return layout.UniformInset(unit.Dp(12)).Layout(gtx, c.Layout)
							}))
						}

						for i, item := range gui.filtered {
							isSelected := i == gui.selectedIdx
							hints = append(hints, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Stack{}.Layout(gtx,
									layout.Expanded(func(gtx layout.Context) layout.Dimensions {
										if !isSelected {
											return layout.Dimensions{}
										}
										paint.FillShape(gtx.Ops, color.NRGBA{R: 60, G: 60, B: 60, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
										return layout.Dimensions{Size: gtx.Constraints.Min}
									}),
									layout.Stacked(func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													if item.Group == "" {
														return layout.Dimensions{}
													}
													return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														return layout.Stack{}.Layout(gtx,
															layout.Expanded(func(gtx layout.Context) layout.Dimensions {
																paint.FillShape(gtx.Ops, color.NRGBA{R: 80, G: 80, B: 100, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
																return layout.Dimensions{Size: gtx.Constraints.Min}
															}),
															layout.Stacked(func(gtx layout.Context) layout.Dimensions {
																c := material.Caption(th, strings.ToUpper(item.Group))
																c.Font.Weight = 700
																c.TextSize = unit.Sp(10)
																c.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
																return layout.UniformInset(unit.Dp(3)).Layout(gtx, c.Layout)
															}),
														)
													})
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													c := material.Caption(th, item.FullTitle)
													c.Font.Weight = 700
													if isSelected {
														c.Color = color.NRGBA{R: 255, G: 200, B: 0, A: 255}
													}
													return c.Layout(gtx)
												}),
												layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													c := material.Caption(th, item.Description)
													c.Color = color.NRGBA{R: 180, G: 180, B: 180, A: 255}
													return c.Layout(gtx)
												}),
											)
										})
									}),
								)
							}))
						}

						if len(hints) == 0 {
							hints = append(hints, layout.Rigid(material.Caption(th, " No results...").Layout))
						}

						return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx, hints...)
						})
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, color.NRGBA{R: 30, G: 30, B: 35, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								ed := material.Editor(th, input, "Type ':' or hit Space for commands...")
								ed.TextSize = unit.Sp(16)
								ed.HintColor = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
								return ed.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.UniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(th, sendButton, "SEND")
									btn.TextSize = unit.Sp(14)
									return btn.Layout(gtx)
								})
							}),
						)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.Caption(th, "Alloy GUI | F1: help | "+fmt.Sprintf("%d messages", len(client.Messages()))).Layout(gtx)
			})
		}),
	)
}

func renderModals(gtx layout.Context, th *material.Theme, client *frontend.Client, gui *guiState) layout.Dimensions {
	if gui.mode == ModeNormal || gui.mode == ModeCommand {
		return layout.Dimensions{}
	}

	paintOverlay(gtx, color.NRGBA{A: 150})

	switch gui.mode {
	case ModeForm:
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
								var children []layout.FlexChild
								children = append(children, layout.Rigid(material.H6(th, strings.ToUpper(gui.form.title)).Layout))

								for i, param := range gui.form.params {
									idx := i
									label := param.Name
									if param.Required {
										label += "*"
									}
									typeHint := " (" + param.Type + ")"
									if param.Type == "enum" {
										typeHint = " [" + strings.Join(param.Choices, "|") + "]"
									}

									children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											ed := material.Editor(th, &gui.form.editors[idx], label+typeHint)
											return ed.Layout(gtx)
										})
									}))

									if len(gui.form.errors) > idx && gui.form.errors[idx] != "" {
										children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											e := material.Caption(th, "(!) "+gui.form.errors[idx])
											e.Color = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
											return layout.UniformInset(unit.Dp(4)).Layout(gtx, e.Layout)
										}))
									}
								}

								children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEvenly}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if gui.form.submit.Clicked(gtx) {
												valid := true
												gui.form.errors = make([]string, len(gui.form.params))
												payloadMap := make(map[string]any)

												for i, param := range gui.form.params {
													val := gui.form.editors[i].Text()

													// Basic validation
													if param.Required && val == "" {
														gui.form.errors[i] = "Required"
														valid = false
														continue
													}

													switch param.Type {
													case "int":
														var v int
														if n, _ := fmt.Sscanf(val, "%d", &v); n == 0 {
															gui.form.errors[i] = "Must be integer"
															valid = false
														} else {
															payloadMap[param.Name] = v
														}
													case "bool":
														low := strings.ToLower(val)
														if low == "true" || low == "1" || low == "y" {
															payloadMap[param.Name] = true
														} else if low == "false" || low == "0" || low == "n" {
															payloadMap[param.Name] = false
														} else {
															gui.form.errors[i] = "Must be true/false"
															valid = false
														}
													case "enum":
														found := false
														for _, c := range param.Choices {
															if val == c {
																found = true
																break
															}
														}
														if !found {
															gui.form.errors[i] = "Invalid choice"
															valid = false
														} else {
															payloadMap[param.Name] = val
														}
													default:
														payloadMap[param.Name] = val
													}
												}

												if valid {
													payload, _ := json.Marshal(payloadMap)
													parts := strings.Fields(gui.form.title)
													if len(parts) >= 2 {
														go client.Send(context.Background(), parts[0], parts[1], payload)
													}
													gui.mode = ModeNormal
												}
											}
											return material.Button(th, &gui.form.submit, "Submit").Layout(gtx)
										}),
										layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if gui.form.cancel.Clicked(gtx) {
												gui.mode = ModeNormal
											}
											return material.Button(th, &gui.form.cancel, "Cancel").Layout(gtx)
										}),
									)
								}))

								return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx, children...)
							})
						}),
					)
				})
			})
		})
	case ModeAiSwitch:
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
								if gui.aiSwitch.cancel.Clicked(gtx) {
									gui.mode = ModeNormal
								}
								return material.Button(th, &gui.aiSwitch.cancel, "Cancel").Layout(gtx)
							}),
						)
					})
				})
			})
		})
	case ModeAiQuery:
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
								if gui.aiQuery.cancel.Clicked(gtx) {
									gui.mode = ModeNormal
								}
								return material.Button(th, &gui.aiQuery.cancel, "Cancel").Layout(gtx)
							}),
						)
					})
				})
			})
		})
	}
	return layout.Dimensions{}
}

func drawDashboard(gtx layout.Context, th *material.Theme, gui *guiState) layout.Dimensions {
	if len(gui.tileOrder) == 0 {
		return layout.Dimensions{}
	}

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var rows []layout.FlexChild

		cols := 2
		if gtx.Constraints.Max.X < 800 {
			cols = 1
		}

		for i := 0; i < len(gui.tileOrder); i += cols {
			end := i + cols
			if end > len(gui.tileOrder) {
				end = len(gui.tileOrder)
			}
			rowTiles := gui.tileOrder[i:end]

			rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var colsItems []layout.FlexChild
				for _, id := range rowTiles {
					tile := gui.dashboardTiles[id]
					colsItems = append(colsItems, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Stack{}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									paint.FillShape(gtx.Ops, color.NRGBA{R: 35, G: 35, B: 35, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
								}),
							)
						})
					}))
				}
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, colsItems...)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
}
