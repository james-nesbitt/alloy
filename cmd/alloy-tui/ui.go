package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) leaderMenuView() string {
	if !m.isLeader || m.commandTree == nil {
		return ""
	}

	node := m.commandTree.Find(m.breadcrumbs)
	if node == nil {
		return ""
	}

	keys := make([]string, 0, len(node.Children))
	for k := range node.Children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var items []string
	for _, k := range keys {
		child := node.Children[k]
		label := k
		desc := child.Description
		if len(child.Children) > 0 {
			desc = "..."
		}

		item := lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).Bold(true).Render(label) + " " +
			lipgloss.NewStyle().Foreground(lipgloss.Color("200")).Render("→") + " " +
			lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(desc)

		items = append(items, item)
	}

	// Calculate column layout
	columnCount := 3
	if len(items) < 5 {
		columnCount = 1
	}

	var rows []string
	for i := 0; i < len(items); i += columnCount {
		rowItems := items[i:min(i+columnCount, len(items))]
		row := ""
		for _, ri := range rowItems {
			row += fmt.Sprintf("%-25s", ri) + "  "
		}
		rows = append(rows, row)
	}

	title := " " + strings.Join(append([]string{"Leader"}, m.breadcrumbs...), " > ") + " "
	titleStyle := lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("255")).Bold(true)

	menuBody := strings.Join(rows, "\n")

	menuStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1)

	return "\n" + titleStyle.Render(title) + "\n" + menuStyle.Render(menuBody)
}

func (m Model) dashboardView() string {
	if len(m.TileOrder) == 0 {
		status := "Connecting to kernel..."
		if len(m.targets) > 0 {
			status = fmt.Sprintf("Connected. Discovered %d plugins. Waiting for dashboard widgets...", len(m.targets))
		}

		return lipgloss.NewStyle().
			Width(m.width).
			Height(m.height-3).
			Align(lipgloss.Center, lipgloss.Center).
			Render(fmt.Sprintf("%s\n\nNo dynamic dashboard widgets registered.\nWaiting for WASM plugins to initialize...", status))
	}

	cols := 2
	if m.width < 100 {
		cols = 1
	}

	tileStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Margin(1, 1).
		Width((m.width / cols) - 4).
		Height((m.height / 3) - 2)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var rows []string
	var currentRow []string

	for i, id := range m.TileOrder {
		tile := m.DashboardTiles[id]

		var content string
		if tile.ContentType == "json" {
			// Pretty print JSON
			var obj interface{}
			if err := json.Unmarshal(tile.RawContent, &obj); err == nil {
				pretty, _ := json.MarshalIndent(obj, "", "  ")
				content = string(pretty)
			} else {
				content = string(tile.RawContent)
			}
		} else {
			// Default to text/markdown list
			if len(tile.Content) > 0 && tile.Content[0] != "" {
				content = strings.Join(tile.Content, "\n")
			} else {
				content = string(tile.RawContent)
			}
		}

		footer := ""
		if len(tile.Actions) > 0 {
			footer = "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("Actions: "+strings.Join(tile.Actions, ", "))
		}

		tileView := tileStyle.Render(
			lipgloss.JoinVertical(lipgloss.Left,
				lipgloss.JoinHorizontal(lipgloss.Top, titleStyle.Render(tile.Title), " ", statusStyle.Render(" "+tile.Status)),
				"",
				contentStyle.Render(content),
				footer,
			),
		)

		currentRow = append(currentRow, tileView)
		if (i+1)%cols == 0 || i == len(m.TileOrder)-1 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, currentRow...))
			currentRow = []string{}
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func formatMessage(msg string) string {
	// Simple helper for generic styling if needed
	return msg
}
