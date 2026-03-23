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
