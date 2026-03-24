package main

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/james-nesbitt/alloy/pkg/frontend"
	"github.com/james-nesbitt/alloy/pkg/frontend/tui"
)

func (m Model) executeFormCommand() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	payloadMap := make(map[string]string)
	for i, field := range m.formFields {
		payloadMap[strings.ToLower(field)] = m.formValues[i]
	}
	payload, _ := json.Marshal(payloadMap)

	parts := strings.Fields(m.formTitle)
	if len(parts) < 2 {
		return m, nil
	}
	target := parts[0]
	method := parts[1]

	cmds = append(cmds, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := m.client.Send(ctx, target, method, payload)
		if err != nil {
			return errMsg(err)
		}
		return messageMsg(resp)
	})

	// Reset form
	m.formFields = nil
	m.formValues = nil
	m.formTitle = ""
	m.formIdx = 0

	return m, tea.Batch(cmds...)
}

func (m Model) executeCommand(cmdStr string) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return m, nil
	}

	if strings.HasPrefix(cmdStr, ":") {
		cmdStr = cmdStr[1:]
	}

	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return m, nil
	}

	target := parts[0]
	method := ""
	if len(parts) > 1 {
		method = parts[1]
	}

	// Update recency
	if m.recency == nil {
		m.recency = make(map[string]int)
	}
	m.recency[target+" "+method] = int(time.Now().Unix())
	m.frequency[target+" "+method]++

	// GUI/Specialized Commands (TUI compatible logic)
	switch target {
	case "project":
		switch method {
		case "open":
			if len(parts) < 3 {
				m.selectType = tui.SelectProject
				m.Mode = tui.ModeCommand
				m.commandInput.Focus()
				return m, m.fetchProjects()
			}
		case "set-workspace", "list-workspaces":
			if len(parts) < 3 {
				m.selectType = tui.SelectWorkspace
				m.Mode = tui.ModeCommand
				m.commandInput.Focus()
				return m, m.fetchWorkspaces()
			}
		}
	case "buffer":
		switch method {
		case "open", "read":
			if len(parts) >= 3 {
				m.activeBuffer = parts[2]
				return m, m.fetchBufferContent(m.activeBuffer)
			}
		}
	}

	// Default: send to client
	payload := ""
	if len(parts) > 2 {
		payload = strings.Join(parts[2:], " ")
	}

	cmds = append(cmds, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := m.client.Send(ctx, target, method, []byte(payload))
		if err != nil {
			return errMsg(err)
		}
		return messageMsg(resp)
	})

	return m, tea.Batch(cmds...)
}

func (m Model) filteredCommands() []CommandOption {
	var results []CommandOption
	input := m.commandInput.Value()

	if m.selectType == tui.SelectProject {
		for _, p := range m.Projects {
			score := frontend.FuzzyScore(p.Name, input)
			if score > 0 {
				results = append(results, CommandOption{
					Raw:         p.ID,
					Display:     p.Name,
					Description: p.Description,
					Score:       score,
				})
			}
		}
	} else if m.selectType == tui.SelectWorkspace {
		for _, w := range m.Workspaces {
			score := frontend.FuzzyScore(w.Name, input)
			if score > 0 {
				results = append(results, CommandOption{
					Raw:         w.ID,
					Display:     w.Name,
					Description: w.Path,
					Score:       score,
				})
			}
		}
	} else if !m.isLeader || (m.isLeader && input != "") {
		// OMNI Mode: search across the entire tree
		if strings.HasPrefix(input, ":") {
			input = input[1:]
		}

		if m.commandTree == nil {
			return nil
		}

		flattened := m.commandTree.Flatten("")
		for _, item := range flattened {
			score := frontend.FuzzyScore(item.FullTitle, input)
			if score > 0 {
				status := "running"
				if s, ok := m.statuses[item.Target]; ok {
					status = s
				}

				// Boost contextual commands
				if m.Mode == tui.ModeChat && (item.Target == "chat" || item.Target == "ai") {
					score += 200
				}
				if m.ActiveProject != nil && (item.Target == "project") {
					score += 50
				}

				results = append(results, CommandOption{
					Raw:         item.Target + " " + item.Method,
					Display:     item.FullTitle,
					Description: item.Description,
					Status:      status,
					Frequency:   m.frequency[item.Target+" "+item.Method],
					Score:       score,
					Annotation:  item.Group,
					Params:      item.Params,
				})
			}
		}
	} else if m.isLeader && input == "" {
		node := m.commandTree.Find(m.breadcrumbs)
		if node != nil {
			var keys []string
			for k := range node.Children {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, k := range keys {
				child := node.Children[k]
				status := "running"
				if s, ok := m.statuses[child.Target]; ok {
					status = s
				}

				results = append(results, CommandOption{
					Raw:         child.Target + " " + child.Method,
					Display:     k,
					Description: child.Description,
					Annotation:  child.Annotation,
					IsDir:       len(child.Children) > 0,
					Method:      child.Method,
					Status:      status,
					Frequency:   m.frequency[child.Target+" "+child.Method],
					Score:       1,
				})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Status != results[j].Status {
			if results[i].Status == "crashed" {
				return false
			}
			if results[j].Status == "crashed" {
				return true
			}
		}
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		ri := m.recency[results[i].Raw]
		rj := m.recency[results[j].Raw]
		if ri != rj {
			return ri > rj
		}
		fi := results[i].Frequency
		fj := results[j].Frequency
		if fi != fj {
			return fi > fj
		}
		return results[i].Display < results[j].Display
	})

	if len(results) > 10 {
		results = results[:10]
	}
	return results
}
