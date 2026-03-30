package tui

import (
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

type DashboardTile = frontend.DashboardTile
type Project = frontend.Project
type Workspace = frontend.Workspace
type Presence = frontend.Presence

type Cursor struct {
	Row      int    `json:"row"`
	Col      int    `json:"col"`
	User     string `json:"user"`
	LastSeen int64  `json:"last_seen"`
}

type DiscoveryMsg struct {
	Targets []api.Registration `json:"targets"`
}

type MessageMsg api.Message
type ErrMsg error
type TickMsg time.Time

type Pane struct {
	Type     int     // ModeNormal, ModeDashboard, ModeChat, ModeEdit
	WidthPct float64 // 0.0 to 1.0
	WidgetID string  // Specific widget for this pane
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
	ModeInspector = 7
	ModeOmni      = 8
)
