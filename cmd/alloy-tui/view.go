package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/james-nesbitt/alloy/pkg/frontend/tui"
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "\n  Initializing screen size..."
	}

	modeStr := " NORMAL "
	modeStyle := lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Bold(true)

	switch m.Mode {
	case tui.ModeInsert:
		modeStr = " INSERT "
		modeStyle = modeStyle.Background(lipgloss.Color("2"))
	case tui.ModeChat:
		modeStr = " CHAT "
		modeStyle = modeStyle.Background(lipgloss.Color("6"))
	case tui.ModeForm:
		modeStr = " FORM "
		modeStyle = modeStyle.Background(lipgloss.Color("13"))
	case tui.ModeDashboard:
		modeStr = " DASHBOARD "
		modeStyle = modeStyle.Background(lipgloss.Color("14"))
	case tui.ModeInspector:
		modeStr = " INSPECTOR "
		modeStyle = modeStyle.Background(lipgloss.Color("1"))
	case tui.ModeEdit:
		modeStr = " EDIT "
		modeStyle = modeStyle.Background(lipgloss.Color("2"))
	case tui.ModeCommand:
		if m.isLeader {
			modeStr = " LEADER "
			modeStyle = modeStyle.Background(lipgloss.Color("3"))
		} else {
			modeStr = " COMMAND "
			modeStyle = modeStyle.Background(lipgloss.Color("5"))
		}
	}

	statusStyle := lipgloss.NewStyle().Background(lipgloss.Color("8")).Foreground(lipgloss.Color("15"))
	projectStr := " No Project "
	if m.ActiveProject != nil {
		projectStr = " Project: " + m.ActiveProject.Name + " "
	}

	remoteStr := ""
	if m.remoteCursors != nil {
		for u := range m.remoteCursors {
			if u == m.client.Actor() {
				continue
			}
			remoteStr += " | " + u
		}
	}

	statusLine := lipgloss.JoinHorizontal(lipgloss.Center,
		modeStyle.Render(modeStr),
		statusStyle.Width(m.width-len(modeStr)-len(projectStr)).Render(fmt.Sprintf(" Buffer: %s | Channel: #%s%s", m.activeBuffer, m.ActiveChannel, remoteStr)),
		modeStyle.Background(lipgloss.Color("12")).Render(projectStr),
	)

	var mainView string
	workingHeight := m.height - 3
	if m.Mode == tui.ModeCommand || m.Mode == tui.ModeForm {
		workingHeight = (m.height * 2) / 3
	}

	if len(m.Panes) > 1 {
		var views []string
		for i, p := range m.Panes {
			paneWidth := int(float64(m.width) * p.WidthPct)
			if i == len(m.Panes)-1 {
				// Fill remaining width
				used := 0
				for j := 0; j < i; j++ {
					used += int(float64(m.width) * m.Panes[j].WidthPct)
				}
				paneWidth = m.width - used
			}

			// Add visual border for focused pane
			style := lipgloss.NewStyle().Width(paneWidth).Height(workingHeight)
			if i == m.FocusIdx {
				style = style.Border(lipgloss.DoubleBorder(), false, true, false, true).BorderForeground(lipgloss.Color("62"))
			} else {
				style = style.Border(lipgloss.NormalBorder(), false, true, false, true).BorderForeground(lipgloss.Color("240"))
			}

			content := m.renderPaneContent(p, paneWidth-2, workingHeight)
			views = append(views, style.Render(content))
		}
		mainView = lipgloss.JoinHorizontal(lipgloss.Top, views...)
	} else {
		mainView = m.renderPaneContent(m.Panes[0], m.width, workingHeight)
	}

	view := lipgloss.JoinVertical(lipgloss.Left,
		mainView,
		statusLine,
	)

	if m.Mode == tui.ModeCommand {
		prompt := ":"
		if m.isLeader {
			prompt = strings.Join(m.breadcrumbs, " > ")
			if prompt != "" {
				prompt += " > "
			}
		}
		m.commandInput.Placeholder = prompt

		// Add Leader Menu or Filtered Commands above the command bar
		if m.isLeader && m.commandInput.Value() == "" {
			view = lipgloss.JoinVertical(lipgloss.Left, view, m.leaderMenuView())
		} else {
			filtered := m.filteredCommands()
			if len(filtered) > 0 {
				// Live Preview Panel
				selected := filtered[m.selectedCmdIdx]

				previewStyle := lipgloss.NewStyle().
					Background(lipgloss.Color("0")).
					Foreground(lipgloss.Color("7")).
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("62")).
					Padding(0, 1).
					Width(m.width - 2)

				// Construct detailed preview
				statusLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("● ONLINE")
				if s, ok := m.statuses[strings.Split(selected.Raw, " ")[0]]; ok && s == "crashed" {
					statusLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("✘ CRASHED")
				}

				previewContent := fmt.Sprintf(
					"%s  %s\n\n%s\n\n%s",
					lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")).Render(selected.Display),
					statusLabel,
					lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(selected.Description),
					lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("Usage: "+selected.Raw),
				)

				view = lipgloss.JoinVertical(lipgloss.Left, view, previewStyle.Render(previewContent))

				listStyle := lipgloss.NewStyle().Background(lipgloss.Color("0")).Foreground(lipgloss.Color("7")).Width(m.width)
				selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Bold(true)
				marginaliaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)

				var rows []string
				lastGroup := ""
				for i, opt := range filtered {
					// Grouping by Annotation (Group)
					group := opt.Annotation
					if group == "" {
						target := opt.Raw
						if idx := strings.Index(opt.Raw, " "); idx != -1 {
							target = opt.Raw[:idx]
						}
						group = target
					}

					if group != lastGroup {
						groupHeader := lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true).Render(" ── " + strings.ToUpper(group) + " ")
						rows = append(rows, groupHeader)
						lastGroup = group
					}

					label := opt.Display
					statusStr := ""
					switch opt.Status {
					case "crashed":
						statusStr = " (CRASHED)"
					case "error":
						statusStr = " (ERROR)"
					case "loading":
						statusStr = " (LOADING...)"
					case "registered":
						statusStr = " (LAZY)"
					}

					var line string
					if m.isLeader {
						annotation := ""
						if opt.Annotation != "" {
							annotation = fmt.Sprintf("[%s] ", opt.Annotation)
						}
						method := opt.Method
						if opt.IsDir {
							method = "..."
						}

						// Multi-column leader style
						left := fmt.Sprintf(" %-2s %s%-15s", opt.Display, annotation, method)
						right := marginaliaStyle.Render(opt.Description + statusStr)

						// Calculate padding
						pad := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
						if pad < 0 {
							pad = 0
						}
						line = left + strings.Repeat(" ", pad) + right
					} else {
						// standard command style
						left := " " + label
						right := marginaliaStyle.Render(opt.Description + statusStr)

						pad := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
						if pad < 0 {
							pad = 0
						}
						line = left + strings.Repeat(" ", pad) + right
					}

					if i == m.selectedCmdIdx {
						rows = append(rows, selectedStyle.Width(m.width).Render(line))
					} else {
						switch opt.Status {
						case "crashed", "error":
							rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(line))
						case "loading":
							rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(line))
						case "registered":
							rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(line))
						default:
							rows = append(rows, line)
						}
					}
				}

				listStr := "\n" + strings.Join(rows, "\n")
				view = lipgloss.JoinVertical(lipgloss.Left, view, listStyle.Render(listStr))
			}
		}
		view = lipgloss.JoinVertical(lipgloss.Left, view, m.commandInput.View())
	} else if m.Mode == tui.ModeForm {
		formStyle := lipgloss.NewStyle().Background(lipgloss.Color("0")).Foreground(lipgloss.Color("7")).Width(m.width)
		labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))

		var rows []string
		rows = append(rows, lipgloss.NewStyle().Bold(true).Underline(true).Render(strings.ToUpper(m.formTitle)))

		for i, param := range m.formParams {
			val := ""
			if i < m.formIdx {
				val = m.formValues[i]
			}

			prefix := "  "
			if i == m.formIdx {
				prefix = "> "
			}

			reqStr := ""
			if param.Required {
				reqStr = "*"
			}

			typeStr := " (" + param.Type + ")"
			if param.Type == "enum" {
				typeStr = " [" + strings.Join(param.Choices, "|") + "]"
			}

			line := prefix + labelStyle.Render(param.Name+reqStr) + typeStr + ": "
			if i == m.formIdx {
				line += m.commandInput.Value()
				if m.formError != "" {
					line += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("(!) "+m.formError)
				}
			} else {
				line += val
			}
			rows = append(rows, line)
		}

		listStr := "\n" + strings.Join(rows, "\n")
		view = lipgloss.JoinVertical(lipgloss.Left, view, formStyle.Render(listStr))
		view = lipgloss.JoinVertical(lipgloss.Left, view, m.commandInput.View())
	} else if m.Mode == tui.ModeOmni {
		view = lipgloss.JoinVertical(lipgloss.Left, view, m.omniPaletteView())
		view = lipgloss.JoinVertical(lipgloss.Left, view, m.commandInput.View())
	}

	return view
}

func (m Model) omniPaletteView() string {
	paletteStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("0")).
		Foreground(lipgloss.Color("7")).
		Width(m.width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62"))

	return paletteStyle.Render(m.omniList.View())
}

func (m Model) renderPaneContent(p tui.Pane, width int, height int) string {
	if p.WidgetID != "" {
		if tile, ok := m.DashboardTiles[p.WidgetID]; ok {
			return lipgloss.NewStyle().Width(width).Height(height).Render(
				lipgloss.JoinVertical(lipgloss.Left,
					lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")).Render(" "+tile.Title),
					lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" " + strings.Repeat("─", width-2)),
					lipgloss.NewStyle().Padding(0, 1).Render(string(tile.RawContent)),
				),
			)
		}
	}

	paneType := p.Type
	if paneType == tui.ModeInsert || paneType == tui.ModeChat || paneType == tui.ModeEdit {
		m.textarea.SetWidth(width)
		m.textarea.SetHeight(height)
		return m.textarea.View()
	} else if paneType == tui.ModeDashboard {
		oldW, oldH := m.width, m.height
		m.width, m.height = width, height+3
		view := m.dashboardView()
		m.width, m.height = oldW, oldH
		return view
	} else if paneType == tui.ModeInspector {
		m.inspectorVp.Width = width
		m.inspectorVp.Height = height
		return m.inspectorVp.View()
	}

	m.viewport.Width = width
	m.viewport.Height = height
	return m.viewport.View()
}
