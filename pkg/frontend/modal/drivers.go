package modal

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
	switch state.Mode {
	case ModeNormal:
		return h.handleNormal(key, state)
	case ModeInsert:
		return h.handleInsert(key, state)
	default:
		return nil, false
	}
}

func (h *HelixDriver) handleNormal(key Key, state *State) (Intent, bool) {
	switch key.Code {
	case "i":
		state.Mode = ModeInsert
		return ModeIntent{NewMode: ModeInsert}, true
	case "j":
		// Helix: j selects the next line (movement is a selection change)
		return MoveIntent{Direction: "down", Count: 1}, true
	case "k":
		return MoveIntent{Direction: "up", Count: 1}, true
	case "h":
		return MoveIntent{Direction: "left", Count: 1}, true
	case "l":
		return MoveIntent{Direction: "right", Count: 1}, true
	case "w":
		// select next word
		return MoveIntent{Direction: "word-forward", Count: 1}, true
	case "x":
		// select current line
		return ActionIntent{Verb: "select-line", Selection: state.Selection}, true
	case "d":
		// delete selection (selection must exist)
		return ActionIntent{Verb: "delete", Selection: state.Selection}, true
	}
	return nil, false
}

func (h *HelixDriver) handleInsert(key Key, state *State) (Intent, bool) {
	if key.Code == "esc" {
		state.Mode = ModeNormal
		return ModeIntent{NewMode: ModeNormal}, true
	}
	return nil, false
}

// MeowDriver implements the Layout-first philosophy.
type MeowDriver struct {
	modes []Mode
}

func NewMeowDriver() *MeowDriver {
	return &MeowDriver{
		modes: []Mode{ModeNormal, ModeInsert, ModeSelection},
	}
}

func (m *MeowDriver) Name() string { return "meow" }

func (m *MeowDriver) Modes() []Mode { return m.modes }

func (m *MeowDriver) Handle(key Key, state *State) (Intent, bool) {
	// Meow uses positional keys like 'h', 'j', 'k', 'l' for movement 
	// but often maps them to physical locations.
	switch state.Mode {
	case ModeNormal:
		return m.handleNormal(key, state)
	case ModeInsert:
		return m.handleInsert(key, state)
	default:
		return nil, false
	}
}

func (m *MeowDriver) handleNormal(key Key, state *State) (Intent, bool) {
	switch key.Code {
	case "i":
		state.Mode = ModeInsert
		return ModeIntent{NewMode: ModeInsert}, true
	// Meow specific movements/selections
	case "n": // next
		return MoveIntent{Direction: "down", Count: 1}, true
	case "p": // previous
		return MoveIntent{Direction: "up", Count: 1}, true
	case "esc":
		// usually stays in normal or switches to keypad
		return nil, false
	}
	return nil, false
}

func (m *MeowDriver) handleInsert(key Key, state *State) (Intent, bool) {
	if key.Code == "esc" {
		state.Mode = ModeNormal
		return ModeIntent{NewMode: ModeNormal}, true
	}
	return nil, false
}
