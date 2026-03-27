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

func (v *VimDriver) Handle(key Key, state *State) Result {
	switch state.Mode {
	case ModeNormal:
		return v.handleNormal(key, state)
	case ModeInsert:
		return v.handleInsert(key, state)
	case ModeSelection: // Visual mode in Vim
		return v.handleSelection(key, state)
	case ModeCommand:
		return v.handleCommand(key, state)
	default:
		return Result{Consumed: false}
	}
}

func (v *VimDriver) handleCommand(key Key, state *State) Result {
	if key.Code == "esc" {
		state.Mode = ModeNormal
		return Result{Intent: ModeIntent{NewMode: ModeNormal}, Consumed: true}
	}
	// Consume all keys in command mode as InputIntent
	if !key.Ctrl && !key.Alt {
		return Result{Intent: InputIntent{Text: key.Code}, Consumed: true}
	}
	return Result{Consumed: false}
}

func (v *VimDriver) handleNormal(key Key, state *State) Result {
	count := state.Count
	if count == 0 {
		count = 1
	}

	// Handle pending sequences (e.g., 'g', 'd')
	if len(state.Pending) > 0 {
		first := state.Pending[0].Code
		switch first {
		case "g":
			if key.Code == "g" {
				return Result{Intent: MoveIntent{Direction: "buffer-start", Count: count}, Consumed: true}
			}
			// Other 'g' sequences like 'ge', 'g$', etc.
		case "d":
			if key.Code == "d" {
				return Result{Intent: ActionIntent{Verb: "delete-line", Selection: state.Selection}, Consumed: true}
			}
		case "ctrl+w": // Wait for 'v', 's', 'l', 'h', etc.
			switch key.Code {
			case "v":
				return Result{Intent: WindowIntent{Action: "split-v"}, Consumed: true}
			case "s":
				return Result{Intent: WindowIntent{Action: "split-h"}, Consumed: true}
			case "h", "left":
				return Result{Intent: WindowIntent{Action: "focus-left"}, Consumed: true}
			case "l", "right":
				return Result{Intent: WindowIntent{Action: "focus-right"}, Consumed: true}
			case "j", "down":
				return Result{Intent: WindowIntent{Action: "focus-down"}, Consumed: true}
			case "k", "up":
				return Result{Intent: WindowIntent{Action: "focus-up"}, Consumed: true}
			case "c", "q":
				return Result{Intent: WindowIntent{Action: "close"}, Consumed: true}
			}
		}
		return Result{Consumed: false} // Unknown sequence, don't consume and let engine clear pending
	}

	// Handle digits for count
	if key.Code >= "0" && key.Code <= "9" && !key.Ctrl && !key.Alt {
		if key.Code == "0" && state.Count == 0 {
			// '0' is "line-start" if no count is being built
			return Result{Intent: MoveIntent{Direction: "line-start", Count: 1}, Consumed: true}
		}
		digit := int(key.Code[0] - '0')
		state.Count = state.Count*10 + digit
		return Result{Consumed: true}
	}

	// Direct Ctrl sequences
	if key.Ctrl {
		switch key.Code {
		case "w":
			return Result{Consumed: true, Incomplete: true}
		case "s":
			return Result{Intent: BufferIntent{Action: "save"}, Consumed: true}
		case "u":
			return Result{Intent: MoveIntent{Direction: "half-page-up", Count: 1}, Consumed: true}
		case "d":
			return Result{Intent: MoveIntent{Direction: "half-page-down", Count: 1}, Consumed: true}
		}
	}

	switch key.Code {
	case "i":
		state.Mode = ModeInsert
		return Result{Intent: ModeIntent{NewMode: ModeInsert}, Consumed: true}
	case "v":
		state.Mode = ModeSelection
		return Result{Intent: ModeIntent{NewMode: ModeSelection}, Consumed: true}
	case "j", "down":
		return Result{Intent: MoveIntent{Direction: "down", Count: count}, Consumed: true}
	case "k", "up":
		return Result{Intent: MoveIntent{Direction: "up", Count: count}, Consumed: true}
	case "h", "left":
		return Result{Intent: MoveIntent{Direction: "left", Count: count}, Consumed: true}
	case "l", "right":
		return Result{Intent: MoveIntent{Direction: "right", Count: count}, Consumed: true}
	case "0":
		return Result{Intent: MoveIntent{Direction: "line-start", Count: 1}, Consumed: true}
	case "$":
		return Result{Intent: MoveIntent{Direction: "line-end", Count: 1}, Consumed: true}
	case "g", "d":
		// Enter incomplete sequence
		return Result{Consumed: true, Incomplete: true}
	case "G":
		return Result{Intent: MoveIntent{Direction: "buffer-end", Count: count}, Consumed: true}
	case ":":
		state.Mode = ModeCommand
		return Result{Intent: ModeIntent{NewMode: ModeCommand}, Consumed: true}
	case "/", "?":
		return Result{Intent: SearchIntent{Type: "regex"}, Consumed: true}
	case "u":
		return Result{Intent: ActionIntent{Verb: "undo"}, Consumed: true}
	case "ctrl+r":
		return Result{Intent: ActionIntent{Verb: "redo"}, Consumed: true}
	case "backspace", "delete":
		return Result{Intent: ActionIntent{Verb: "delete-char"}, Consumed: true}
	case "home":
		return Result{Intent: MoveIntent{Direction: "line-start", Count: 1}, Consumed: true}
	case "end":
		return Result{Intent: MoveIntent{Direction: "line-end", Count: 1}, Consumed: true}
	case "pgup":
		return Result{Intent: MoveIntent{Direction: "page-up", Count: 1}, Consumed: true}
	case "pgdown":
		return Result{Intent: MoveIntent{Direction: "page-down", Count: 1}, Consumed: true}
	}
	return Result{Consumed: false}
}

func (v *VimDriver) handleInsert(key Key, state *State) Result {
	// Let application handle raw typing unless it's Esc
	if key.Code == "esc" {
		state.Mode = ModeNormal
		return Result{Intent: ModeIntent{NewMode: ModeNormal}, Consumed: true}
	}
	// Emit InputIntent for characters
	if !key.Ctrl && !key.Alt && len(key.Code) == 1 {
		return Result{Intent: InputIntent{Text: key.Code}, Consumed: true}
	}
	// Fallback for special keys in insert mode (e.g., enter, backspace)
	if key.Code == "enter" || key.Code == "backspace" || key.Code == "tab" {
		return Result{Intent: InputIntent{Text: key.Code}, Consumed: true}
	}
	return Result{Consumed: false}
}

func (v *VimDriver) handleSelection(key Key, state *State) Result {
	if key.Code == "esc" {
		state.Mode = ModeNormal
		return Result{Intent: ModeIntent{NewMode: ModeNormal}, Consumed: true}
	}
	// Movement during selection
	switch key.Code {
	case "j", "k", "h", "l":
		// Expand the selection based on movement (Implementation detail for the application)
		return Result{Intent: MoveIntent{Direction: "expand", Count: 1}, Consumed: true}
	}
	return Result{Consumed: false}
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

func (h *HelixDriver) Handle(key Key, state *State) Result {
	switch state.Mode {
	case ModeNormal:
		return h.handleNormal(key, state)
	case ModeInsert:
		return h.handleInsert(key, state)
	case ModeCommand:
		return h.handleCommand(key, state)
	default:
		return Result{Consumed: false}
	}
}

func (h *HelixDriver) handleCommand(key Key, state *State) Result {
	if key.Code == "esc" {
		state.Mode = ModeNormal
		return Result{Intent: ModeIntent{NewMode: ModeNormal}, Consumed: true}
	}
	// Consume all keys in command mode as InputIntent
	if !key.Ctrl && !key.Alt {
		return Result{Intent: InputIntent{Text: key.Code}, Consumed: true}
	}
	return Result{Consumed: false}
}

func (h *HelixDriver) handleNormal(key Key, state *State) Result {
	count := state.Count
	if count == 0 {
		count = 1
	}

	// Handle pending sequences (e.g., 'g', 'z', 'm', 'v')
	if len(state.Pending) > 0 {
		first := state.Pending[0].Code
		switch first {
		case "g":
			switch key.Code {
			case "g":
				return Result{Intent: MoveIntent{Direction: "buffer-start", Count: count}, Consumed: true}
			case "e":
				return Result{Intent: MoveIntent{Direction: "buffer-end", Count: count}, Consumed: true}
			case "h":
				return Result{Intent: MoveIntent{Direction: "line-start", Count: 1}, Consumed: true}
			case "l":
				return Result{Intent: MoveIntent{Direction: "line-end", Count: 1}, Consumed: true}
			case "d":
				return Result{Intent: ActionIntent{Verb: "goto-definition"}, Consumed: true}
			}
		case "z":
			switch key.Code {
			case "t":
				return Result{Intent: WindowIntent{Action: "scroll-top"}, Consumed: true}
			case "c":
				return Result{Intent: WindowIntent{Action: "scroll-center"}, Consumed: true}
			case "b":
				return Result{Intent: WindowIntent{Action: "scroll-bottom"}, Consumed: true}
			}
		case "v":
			switch key.Code {
			case "h":
				return Result{Intent: ActionIntent{Verb: "view-split-h"}, Consumed: true}
			case "v":
				return Result{Intent: ActionIntent{Verb: "view-split-v"}, Consumed: true}
			}
		case "space":
			switch key.Code {
			case "f":
				return Result{Intent: BufferIntent{Action: "fuzzy-find"}, Consumed: true}
			case "b":
				return Result{Intent: BufferIntent{Action: "list-buffers"}, Consumed: true}
			case "w":
				return Result{Intent: WindowIntent{Action: "picker"}, Consumed: true}
			}
		}
		// Reset sequence if no match
		return Result{Consumed: false}
	}

	// Handle digits for count
	if key.Code >= "0" && key.Code <= "9" && !key.Ctrl && !key.Alt {
		digit := int(key.Code[0] - '0')
		state.Count = state.Count*10 + digit
		return Result{Consumed: true}
	}

	switch key.Code {
	case "i":
		state.Mode = ModeInsert
		return Result{Intent: ModeIntent{NewMode: ModeInsert}, Consumed: true}
	case "a":
		// append (move right then insert)
		return Result{Intent: ActionIntent{Verb: "append"}, Consumed: true}
	case "j", "down":
		return Result{Intent: MoveIntent{Direction: "down", Count: count}, Consumed: true}
	case "k", "up":
		return Result{Intent: MoveIntent{Direction: "up", Count: count}, Consumed: true}
	case "h", "left":
		return Result{Intent: MoveIntent{Direction: "left", Count: count}, Consumed: true}
	case "l", "right":
		return Result{Intent: MoveIntent{Direction: "right", Count: count}, Consumed: true}
	case "w":
		return Result{Intent: MoveIntent{Direction: "word-forward", Count: count}, Consumed: true}
	case "b":
		return Result{Intent: MoveIntent{Direction: "word-backward", Count: count}, Consumed: true}
	case "e":
		return Result{Intent: MoveIntent{Direction: "word-end", Count: count}, Consumed: true}
	case "x":
		return Result{Intent: ActionIntent{Verb: "select-line", Selection: state.Selection}, Consumed: true}
	case "d":
		return Result{Intent: ActionIntent{Verb: "delete", Selection: state.Selection}, Consumed: true}
	case "c":
		state.Mode = ModeInsert
		return Result{Intent: ActionIntent{Verb: "change", Selection: state.Selection}, Consumed: true}
	case "y":
		return Result{Intent: ActionIntent{Verb: "yank", Selection: state.Selection}, Consumed: true}
	case "p":
		return Result{Intent: ActionIntent{Verb: "paste-after"}, Consumed: true}
	case "P":
		return Result{Intent: ActionIntent{Verb: "paste-before"}, Consumed: true}
	case "u":
		return Result{Intent: ActionIntent{Verb: "undo"}, Consumed: true}
	case "U":
		return Result{Intent: ActionIntent{Verb: "redo"}, Consumed: true}
	case "g", "z", "v", "space":
		return Result{Consumed: true, Incomplete: true}
	case ":":
		state.Mode = ModeCommand
		return Result{Intent: ModeIntent{NewMode: ModeCommand}, Consumed: true}
	case "esc":
		// Clear selection / count
		state.Count = 0
		return Result{Intent: ActionIntent{Verb: "clear-selection"}, Consumed: true}
	}
	return Result{Consumed: false}
}

func (h *HelixDriver) handleInsert(key Key, state *State) Result {
	if key.Code == "esc" {
		state.Mode = ModeNormal
		return Result{Intent: ModeIntent{NewMode: ModeNormal}, Consumed: true}
	}
	// Emit InputIntent for characters
	if !key.Ctrl && !key.Alt && (len(key.Code) == 1 || key.Code == "enter" || key.Code == "backspace" || key.Code == "tab") {
		return Result{Intent: InputIntent{Text: key.Code}, Consumed: true}
	}
	return Result{Consumed: false}
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

func (m *MeowDriver) Handle(key Key, state *State) Result {
	// Meow uses positional keys like 'h', 'j', 'k', 'l' for movement
	// but often maps them to physical locations.
	switch state.Mode {
	case ModeNormal:
		return m.handleNormal(key, state)
	case ModeInsert:
		return m.handleInsert(key, state)
	default:
		return Result{Consumed: false}
	}
}

func (m *MeowDriver) handleNormal(key Key, state *State) Result {
	switch key.Code {
	case "i":
		state.Mode = ModeInsert
		return Result{Intent: ModeIntent{NewMode: ModeInsert}, Consumed: true}
	// Meow specific movements/selections
	case "n": // next
		return Result{Intent: MoveIntent{Direction: "down", Count: 1}, Consumed: true}
	case "p": // previous
		return Result{Intent: MoveIntent{Direction: "up", Count: 1}, Consumed: true}
	case "esc":
		// usually stays in normal or switches to keypad
		return Result{Consumed: false} // Not consumed
	}
	return Result{Consumed: false}
}

func (m *MeowDriver) handleInsert(key Key, state *State) Result {
	if key.Code == "esc" {
		state.Mode = ModeNormal
		return Result{Intent: ModeIntent{NewMode: ModeNormal}, Consumed: true}
	}
	return Result{Consumed: false}
}
