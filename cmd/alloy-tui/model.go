package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
	"github.com/james-nesbitt/alloy/pkg/frontend/tui"
)

// Model represents the overall UI state.
type Model struct {
	client   *frontend.Client
	messages []string
	viewport viewport.Model
	textarea textarea.Model
	targets  []api.Registration
	err      error
	width    int
	height   int
	ready    bool
	msgCh    chan api.Message

	// Modal interface state
	Mode            int
	commandInput    textarea.Model
	activeBuffer    string
	ActiveChannel   string
	isLeader        bool
	breadcrumbs     []string
	subscriptions   map[string]bool
	commandTree     *frontend.CommandNode
	recency         map[string]int
	frequency       map[string]int
	statuses        map[string]string
	selectedCmdIdx  int
	leaderMenuWidth int

	ActiveProject *frontend.Project
	Projects      []frontend.Project
	Workspaces    []frontend.Workspace
	selectType    int

	// Form state
	formTitle  string
	formFields []string
	formValues []string
	formIdx    int

	// Dashboard state
	DashboardTiles map[string]frontend.DashboardTile
	TileOrder      []string

	lastMainMode       int
	localBufferVersion int
	isLocalBufferDirty bool
	remoteCursors      map[string]tui.Cursor

	// Multi-pane state
	Panes    []tui.Pane
	FocusIdx int

	startupTicks int
}

type CommandOption struct {
	Raw         string
	Display     string
	Description string
	Annotation  string
	IsDir       bool
	Method      string
	Status      string
	Frequency   int
	Score       int
	Params      []string
}

type discoveryMsg struct {
	Targets []api.Registration `json:"targets"`
}

type messageMsg api.Message
type errMsg error
type tickMsg time.Time

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.listenForMessages(),
		m.doDiscovery, // Immediate discovery
		tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m Model) listenForMessages() tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-m.msgCh
		if !ok {
			return errMsg(fmt.Errorf("message channel closed"))
		}
		return messageMsg(msg)
	}
}

func NewModel(client *frontend.Client, msgCh chan api.Message) Model {
	ta := textarea.New()
	ta.Placeholder = "Write content here..."
	ta.SetHeight(5)

	ci := textarea.New()
	ci.Placeholder = ":"
	ci.SetHeight(1)

	return Model{
		client:        client,
		textarea:      ta,
		commandInput:  ci,
		msgCh:         msgCh,
		ActiveChannel: "general",
		Mode:          tui.ModeDashboard,
		subscriptions: make(map[string]bool),
		recency:       make(map[string]int),
		frequency:     make(map[string]int),
		statuses:      make(map[string]string),
		DashboardTiles: make(map[string]frontend.DashboardTile),
		TileOrder:      nil,
		width:         80,
		height:        24,
		ready:         false,
		Panes: []tui.Pane{
			{Type: tui.ModeDashboard, WidthPct: 1.0},
		},
		FocusIdx: 0,
	}
}
