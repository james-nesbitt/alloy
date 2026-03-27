package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
	"github.com/james-nesbitt/alloy/pkg/frontend/modal"
	"github.com/james-nesbitt/alloy/pkg/frontend/tui"
)

// OmniResult represents a result from the universal search
type OmniResult struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Type        string            `json:"type"`
	Score       float64           `json:"score"`
	Shortcut    string            `json:"shortcut,omitempty"`
	Source      string            `json:"source,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type listItem struct {
	res OmniResult
}

func (i listItem) Title() string       { return i.res.Title }
func (i listItem) Description() string { return i.res.Description }
func (i listItem) FilterValue() string { return i.res.Title }

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

	// Modal engine
	ModalEngine *modal.Engine

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

	// Omni state
	omniResults     []OmniResult
	omniSelectedIdx int
	omniList        list.Model
	lastSearchQuery string
	searchTimer     *time.Timer

	ActiveProject *frontend.Project
	Projects      []frontend.Project
	Workspaces    []frontend.Workspace
	selectType    int

	// Form state
	formTitle      string
	formParams     []frontend.ParamInfo
	formValues     []string
	formIdx        int
	formError      string
	formValidation func(index int, value string) error

	// Dashboard state
	DashboardTiles map[string]frontend.DashboardTile
	TileOrder      []string

	inspectorLogs []string
	inspectorVp   viewport.Model

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
	Params      []frontend.ParamInfo
}

type discoveryMsg struct {
	Targets []api.Registration `json:"targets"`
}

type searchDebounceMsg struct {
	Query string
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

	// Default to Vim philosophy
	engine := modal.NewEngine(modal.NewVimDriver())

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Universal Search"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false) // Handle filtering on kernel-side for now

	return Model{
		client:         client,
		textarea:       ta,
		commandInput:   ci,
		msgCh:          msgCh,
		ModalEngine:    engine,
		omniList:       l,
		ActiveChannel:  "general",
		Mode:           tui.ModeDashboard,
		subscriptions:  make(map[string]bool),
		recency:        make(map[string]int),
		frequency:      make(map[string]int),
		statuses:       make(map[string]string),
		DashboardTiles: make(map[string]frontend.DashboardTile),
		TileOrder:      nil,
		width:          80,
		height:         24,
		ready:          false,
		Panes: []tui.Pane{
			{Type: tui.ModeDashboard, WidthPct: 1.0},
		},
		FocusIdx: 0,
	}
}
