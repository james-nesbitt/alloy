package modal

import "fmt"

// VimDriver implements the Verb-Noun / Grammar philosophy.
type VimDriver struct {
	modes []Mode
}

func NewVimDriver() *VimDriver {
	return &VimDriver{
		modes: []Mode{ModeNormal, ModeInsert, ModeSelection, ModeCommand},
	}
}

func (v *VimDriver) Name() string { return "vim" }

func (v *VimDriver) Modes() []Mode { return v.modes }

func (v *VimDriver) Handle(key Key, state *State) (Intent, bool) {
	switch state.Mode {
	case ModeNormal:
		return v.handleNormal(key, state)
	case ModeInsert:
		return v.handleInsert(key, state)
	case ModeSelection: // Visual mode in Vim
		return v.handleSelection(key, state)
	default:
		return nil, false
	}
}

func (v *VimDriver) handleNormal(key Key, state *State) (Intent, bool) {
	switch key.Code {
	case "i":
		state.Mode = ModeInsert
		return ModeIntent{NewMode: ModeInsert}, true
	case "v":
		state.Mode = ModeSelection
		return ModeIntent{NewMode: ModeSelection}, true
	case "j":
		return MoveIntent{Direction: "down", Count: 1}, true
	case "k":
		return MoveIntent{Direction: "up", Count: 1}, true
	case "h":
		return MoveIntent{Direction: "left", Count: 1}, true
	case "l":
		return MoveIntent{Direction: "right", Count: 1}, true
	case ":":
		state.Mode = ModeCommand
		return ModeIntent{NewMode: ModeCommand}, true
	}
	return nil, false
}

func (v *VimDriver) handleInsert(key Key, state *State) (Intent, bool) {
	// Let application handle raw typing unless it's Esc
	if key.Code == "esc" {
		state.Mode = ModeNormal
		return ModeIntent{NewMode: ModeNormal}, true
	}
	return nil, false
}

func (v *VimDriver) handleSelection(key Key, state *State) (Intent, bool) {
	if key.Code == "esc" {
		state.Mode = ModeNormal
		return ModeIntent{NewMode: ModeNormal}, true
	}
	// Movement during selection
	switch key.Code {
	case "j", "k", "h", "l":
		// Expand the selection based on movement (Implementation detail for the application)
		return MoveIntent{Direction: "expand", Count: 1}, true
	}
	return nil, false
}

// HelixDriver implements the Object-Verb / Selection-first philosophy.
type HelixDriver struct {
	modes []Mode
}

func NewHelixDriver() *HelixDriver {
	return &HelixDriver{
		modes: []Mode{ModeNormal, ModeInsert, ModeSelection},
	}
}

func (h *HelixDriver) Name() string { return "helix" }

func (h *HelixDriver) Modes() []Mode { return h.modes }

func (h *HelixDriver) Handle(key Key, state *State) (Intent, bool) {
	// Implementation placeholder for Helix:
	// Selection is always active, j/k/h/l expand selection by default.
	return nil, false
}

// MeowDriver implementation placeholder...
