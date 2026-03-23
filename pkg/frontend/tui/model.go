package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

// Model Layout components

type Model struct {
	Client   *frontend.Client
	messages []string
	Viewport viewport.Model
	Textarea textarea.Model
	targets  []frontend.Registration
	Err      error
	Width    int
	Height   int
	Ready    bool
	MsgCh    chan api.Message

	// Modal interface state
	Mode            int
	CommandInput    textarea.Model
	ActiveBuffer    string
	ActiveChannel   string
	IsLeader        bool
	Breadcrumbs     []string
	Subscriptions   map[string]bool
	CommandTree     *frontend.CommandNode
	Recency         map[string]int
	Frequency       map[string]int
	Statuses        map[string]string
	SelectedCmdIdx  int
	LeaderMenuWidth int

	ActiveProject *Project
	Projects      []Project
	Workspaces    []Workspace
	SelectType    int

	// Form state
	FormTitle  string
	FormFields []string
	FormValues []string
	FormIdx    int

	// Dashboard state
	DashboardTiles map[string]DashboardTile
	TileOrder      []string

	LastMainMode       int
	LocalBufferVersion int
	IsLocalBufferDirty bool
	RemoteCursors      map[string]Cursor
}

const (
	SelectNone = iota
	SelectProject
	SelectWorkspace
)

const (
	ModeNormal    = 0
	ModeInsert    = 1
	ModeCommand   = 2
	ModeChat      = 3
	ModeForm      = 4
	ModeDashboard = 5
	ModeEdit      = 6
)

type DashboardTile struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Content   []string `json:"content"`
	Status    string   `json:"status"`
	Actions   []string `json:"actions"`
	Timestamp int64    `json:"timestamp"`
}

type WorkspaceConfig struct {
	DefaultMode string `json:"default_mode"`
	Dashboard   struct {
		Tiles []struct {
			Plugin string `json:"plugin"`
			Weight int    `json:"weight"`
		} `json:"tiles"`
	} `json:"dashboard"`
}

type Project struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Layout      WorkspaceConfig `json:"layout,omitempty"`
}

type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Root string `json:"root"`
}

type Presence struct {
	User      string `json:"user"`
	Status    string `json:"status"`
	LastSeen  int64  `json:"last_seen"`
	Client    string `json:"client"`
	ProjectID string `json:"project_id,omitempty"`
}

type Cursor struct {
	Row      int    `json:"row"`
	Col      int    `json:"col"`
	User     string `json:"user"`
	LastSeen int64  `json:"last_seen"`
}

type DiscoveryMsg struct {
	Targets []frontend.Registration `json:"targets"`
}

type MessageMsg api.Message
type ErrMsg error
type TickMsg time.Time

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.ListenForMessages(),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return TickMsg(t) }),
	)
}

func (m Model) ListenForMessages() tea.Cmd {
	return func() tea.Msg {
		msg := <-m.MsgCh
		return MessageMsg(msg)
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
		Client:        client,
		Textarea:      ta,
		CommandInput:  ci,
		MsgCh:         msgCh,
		ActiveChannel: "general",
		Mode:          ModeDashboard,
		Subscriptions: make(map[string]bool),
		Recency:       make(map[string]int),
		DashboardTiles: map[string]DashboardTile{
			"team": {
				Title:   "Team Presence",
				Content: []string{"● You (Online)", "○ James (Away)", "● AI Worker (Idle)"},
				Status:  "Active",
				Actions: []string{"Invite", "Call"},
			},
			"ai": {
				Title:   "AI Assistant",
				Content: []string{"● Ollama (Running)", "○ Active Model: llama3", "Tasks: 0/1 completed"},
				Actions: []string{"Query", "Summarize"},
			},
			"chat": {
				Title:   "Team Chat",
				Content: []string{"#general", "<James> Anyone online?", "<AI> Ready to help."},
				Status:  "2 unread",
				Actions: []string{"Open", "Clear"},
			},
			"project": {
				Title:   "Current Project",
				Content: []string{"Phase: 5 (Team Collaboration)", "Branch: feature/phase-5-uipolish", "Health: Stable"},
				Status:  "OK",
			},
		},
		TileOrder: []string{"ai", "chat", "project"},
	}
}
