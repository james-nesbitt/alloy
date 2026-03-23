package main

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"image"
	"strings"
	"testing"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

func TestDrawDashboard(t *testing.T) {
	gtx := layout.Context{
		Ops: new(op.Ops),
		Constraints: layout.Constraints{
			Max: image.Pt(800, 600),
		},
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	th := material.NewTheme()

	state := guiState{
		dashboardTiles: make(map[string]frontend.DashboardTile),
		tileOrder:      []string{"test"},
	}
	state.dashboardTiles["test"] = frontend.DashboardTile{
		Title:   "Test Tile",
		Content: []string{"Line 1", "Line 2"},
		Status:  "OK",
	}

	dims := drawDashboard(gtx, th, &state)

	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Errorf("Expected positive dimensions, got %v", dims.Size)
	}
}

func TestFormatMessage(t *testing.T) {
	msg := api.Message{
		Sender: "test",
		Method: "hello",
		Payload: []byte(`"world"`),
		Timestamp: 1234567890,
	}

	formatted := formatMessage(msg)
	expected := "test: \"world\""
	if !strings.Contains(formatted, expected) {
		t.Errorf("Expected message to contain %q, got %q", expected, formatted)
	}
}
