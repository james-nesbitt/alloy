package modal

import "fmt"

// IntentBuilder is a function that creates an Intent based on the current modal state (e.g., using Count).
type IntentBuilder func(state *State) Intent

// MapDriver is a generic modal driver that uses keymaps for its logic.
type MapDriver struct {
	driverName string
	modes      []Mode
	keymaps    map[Mode]map[string]IntentBuilder
	sequences  map[Mode]map[string]bool // True if the key is a prefix for a sequence
}

func (m *MapDriver) Name() string { return m.driverName }

func (m *MapDriver) Modes() []Mode { return m.modes }

func (m *MapDriver) Customize(mode Mode, key string, builder IntentBuilder) {
	if m.keymaps[mode] == nil {
		m.keymaps[mode] = make(map[string]IntentBuilder)
	}
	m.keymaps[mode][key] = builder
}

func (m *MapDriver) Handle(key Key, state *State) Result {
	modeMaps := m.keymaps[state.Mode]
	if modeMaps == nil {
		return Result{Consumed: false}
	}

	keyStr := key.String()

	// If there's a pending sequence, prepend it to the current key
	fullKey := keyStr
	if len(state.Pending) > 0 {
		prefix := ""
		for _, p := range state.Pending {
			prefix += p.String() + " "
		}
		fullKey = prefix + keyStr
	}

	// 1. Check for digits for count (only in normal mode by default)
	if state.Mode == ModeNormal && key.Code >= "0" && key.Code <= "9" && !key.Ctrl && !key.Alt {
		if key.Code == "0" && state.Count == 0 {
			// '0' is usually a movement if no count is built
		} else {
			digit := int(key.Code[0] - '0')
			state.Count = state.Count*10 + digit
			return Result{Consumed: true}
		}
	}

	// 2. Check for exact match in current mode mapping
	if builder, ok := modeMaps[fullKey]; ok {
		intent := builder(state)
		return Result{Intent: intent, Consumed: true}
	}

	// 3. Check if this is a prefix for a longer sequence
	if m.sequences[state.Mode][fullKey] {
		return Result{Consumed: true, Incomplete: true}
	}

	// 4. Default behaviors / fallbacks
	if state.Mode == ModeInsert {
		// Emit InputIntent for characters
		if !key.Ctrl && !key.Alt && (len(key.Code) == 1 || key.Code == "enter" || key.Code == "backspace" || key.Code == "tab") {
			return Result{Intent: InputIntent{Text: key.Code}, Consumed: true}
		}
	} else if state.Mode == ModeCommand {
		if key.Code == "esc" {
			return Result{Intent: ModeIntent{NewMode: ModeNormal}, Consumed: true}
		}
		if !key.Ctrl && !key.Alt {
			return Result{Intent: InputIntent{Text: key.Code}, Consumed: true}
		}
	}

	return Result{Consumed: false}
}

// NewVimDriver creates a Vim modal driver with default bindings.
func NewVimDriver() *MapDriver {
	d := &MapDriver{
		driverName: "vim",
		modes:      []Mode{ModeNormal, ModeInsert, ModeSelection, ModeCommand},
		keymaps:    make(map[Mode]map[string]IntentBuilder),
		sequences:  make(map[Mode]map[string]bool),
	}

	// Initialize maps
	for _, m := range d.modes {
		d.keymaps[m] = make(map[string]IntentBuilder)
		d.sequences[m] = make(map[string]bool)
	}

	// Normal Mode Bindings
	n := d.keymaps[ModeNormal]
	n["i"] = func(s *State) Intent { return ModeIntent{NewMode: ModeInsert} }
	n["v"] = func(s *State) Intent { return ModeIntent{NewMode: ModeSelection} }
	n[":"] = func(s *State) Intent { return ModeIntent{NewMode: ModeCommand} }
	n["u"] = func(s *State) Intent { return ActionIntent{Verb: "undo"} }
	n["ctrl+r"] = func(s *State) Intent { return ActionIntent{Verb: "redo"} }

	// Movements
	move := func(dir string) IntentBuilder {
		return func(s *State) Intent {
			count := s.Count
			if count == 0 {
				count = 1
			}
			return MoveIntent{Direction: dir, Count: count}
		}
	}
	n["j"] = move("down")
	n["down"] = move("down")
	n["k"] = move("up")
	n["up"] = move("up")
	n["h"] = move("left")
	n["left"] = move("left")
	n["l"] = move("right")
	n["right"] = move("right")
	n["w"] = move("word-forward")
	n["b"] = move("word-backward")
	n["e"] = move("word-end")
	n["0"] = move("line-start")
	n["$"] = move("line-end")
	n["home"] = move("line-start")
	n["end"] = move("line-end")
	n["pgup"] = move("page-up")
	n["pgdown"] = move("page-down")
	n["G"] = move("buffer-end")

	// Sequences
	d.sequences[ModeNormal]["g"] = true
	n["g g"] = move("buffer-start")

	d.sequences[ModeNormal]["d"] = true
	n["d d"] = func(s *State) Intent { return ActionIntent{Verb: "delete-line"} }

	// Search
	n["/"] = func(s *State) Intent { return SearchIntent{Type: "regex"} }
	n["?"] = func(s *State) Intent { return SearchIntent{Type: "regex"} }

	// Insert Mode Bindings
	d.keymaps[ModeInsert]["esc"] = func(s *State) Intent { return ModeIntent{NewMode: ModeNormal} }

	// Selection Mode Bindings
	sel := d.keymaps[ModeSelection]
	sel["esc"] = func(s *State) Intent { return ModeIntent{NewMode: ModeNormal} }
	for _, k := range []string{"j", "k", "h", "l", "down", "up", "left", "right"} {
		sel[k] = func(s *State) Intent { return MoveIntent{Direction: "expand", Count: 1} }
	}

	// Command Mode Bindings
	d.keymaps[ModeCommand]["esc"] = func(s *State) Intent { return ModeIntent{NewMode: ModeNormal} }

	return d
}

// NewHelixDriver creates a Helix modal driver with default bindings.
func NewHelixDriver() *MapDriver {
	d := &MapDriver{
		driverName: "helix",
		modes:      []Mode{ModeNormal, ModeInsert, ModeSelection, ModeCommand},
		keymaps:    make(map[Mode]map[string]IntentBuilder),
		sequences:  make(map[Mode]map[string]bool),
	}

	// Initialize
	for _, m := range d.modes {
		d.keymaps[m] = make(map[string]IntentBuilder)
		d.sequences[m] = make(map[string]bool)
	}

	n := d.keymaps[ModeNormal]
	n["i"] = func(s *State) Intent { return ModeIntent{NewMode: ModeInsert} }
	n[":"] = func(s *State) Intent { return ModeIntent{NewMode: ModeCommand} }

	move := func(dir string) IntentBuilder {
		return func(s *State) Intent {
			count := s.Count
			if count == 0 {
				count = 1
			}
			return MoveIntent{Direction: dir, Count: count}
		}
	}
	n["j"] = move("down")
	n["k"] = move("up")
	n["h"] = move("left")
	n["l"] = move("right")
	n["w"] = move("word-forward")
	n["b"] = move("word-backward")
	n["e"] = move("word-end")

	// Actions
	n["x"] = func(s *State) Intent { return ActionIntent{Verb: "select-line"} }
	n["d"] = func(s *State) Intent { return ActionIntent{Verb: "delete"} }
	n["u"] = func(s *State) Intent { return ActionIntent{Verb: "undo"} }
	n["U"] = func(s *State) Intent { return ActionIntent{Verb: "redo"} }

	// Sequences
	d.sequences[ModeNormal]["g"] = true
	n["g g"] = move("buffer-start")
	n["g e"] = move("buffer-end")
	n["g h"] = move("line-start")
	n["g l"] = move("line-end")

	d.sequences[ModeNormal]["space"] = true
	n["space f"] = func(s *State) Intent { return BufferIntent{Action: "fuzzy-find"} }

	// Insert/Command logic handled by MapDriver defaults
	d.keymaps[ModeInsert]["esc"] = func(s *State) Intent { return ModeIntent{NewMode: ModeNormal} }
	d.keymaps[ModeCommand]["esc"] = func(s *State) Intent { return ModeIntent{NewMode: ModeNormal} }

	return d
}

// init registers the default drivers in the global registry
func init() {
	DefaultRegistry.Register(NewVimDriver())
	DefaultRegistry.Register(NewHelixDriver())
}
